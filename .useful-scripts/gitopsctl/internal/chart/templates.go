package chart

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// TemplateViolation is one structural finding against a single Helm template
// file. The File/Line/Rule/Message shape and the FAIL line format mirror the
// MCP manifest lint so CI output stays greppable across gitopsctl commands.
type TemplateViolation struct {
	File    string
	Line    int
	Rule    string
	Message string
}

func (v TemplateViolation) String() string {
	return fmt.Sprintf("FAIL %-46s :%-4d [%s] %s", v.File, v.Line, v.Rule, v.Message)
}

// blockOpeners are the Go template actions that require a matching `end`.
// `else`/`else if` are neutral and deliberately absent.
var blockOpeners = map[string]bool{
	"if":     true,
	"range":  true,
	"with":   true,
	"define": true,
	"block":  true,
}

// templateExtensions are the files a chart's templates/ and charts/*/templates/
// can hold Go template source. `.tpl` holds only named templates.
var templateExtensions = map[string]bool{
	".yaml": true,
	".yml":  true,
	".tpl":  true,
	".txt":  true,
}

// openBlock is one entry of the block stack.
type openBlock struct {
	keyword string
	line    int
}

// CheckTemplateBalance walks every template file under chartDir and reports
// block actions whose `end` is missing, extra, or mismatched.
//
// It exists because `helm lint`/`helm template` only reject a template AFTER a
// balanced-file assumption breaks, and the failure surfaces far from the edit:
// a `{{- if }}` guard whose `{{- end }}` was left behind in a sibling file
// renders as "unexpected {{end}}" at a line in a file nobody touched, while a
// missing `{{- end }}` in a doc that is otherwise valid YAML renders nothing at
// all. Both are silent-until-rendered defects that a static pass catches first.
//
// Only block structure is checked. Action arguments are not parsed, so a
// literal `{{` in embedded prose (which the chart escapes via lit.Brace) can
// never skew the count.
func CheckTemplateBalance(chartDir string) ([]TemplateViolation, int) {
	files, err := templateFiles(chartDir)
	if err != nil {
		return []TemplateViolation{{
			File:    filepath.Base(chartDir),
			Rule:    "walk",
			Message: err.Error(),
		}}, 0
	}

	var violations []TemplateViolation
	for _, file := range files {
		rel := file
		if r, err := filepath.Rel(chartDir, file); err == nil {
			rel = r
		}
		violations = append(violations, checkFile(file, rel)...)
	}
	return violations, len(files)
}

// checkFile balances one template file, reporting every structural error in it
// rather than stopping at the first, so one pass names all of them. path is the
// file to read; name is how it is reported.
func checkFile(path, name string) []TemplateViolation {
	raw, err := os.ReadFile(path)
	if err != nil {
		return []TemplateViolation{{File: name, Rule: "read", Message: err.Error()}}
	}

	var violations []TemplateViolation
	var stack []openBlock
	line := 1

	for i := 0; i < len(raw); i++ {
		switch raw[i] {
		case '\n':
			line++
			continue
		case '{':
			if i+1 >= len(raw) || raw[i+1] != '{' {
				continue
			}
			end := findActionEnd(raw, i+2)
			if end < 0 {
				violations = append(violations, TemplateViolation{
					File: path, Line: line, Rule: "unterminated",
					Message: "`{{` is never closed by `}}`",
				})
				return violations
			}
			keyword := actionKeyword(string(raw[i+2 : end]))
			switch {
			case blockOpeners[keyword]:
				stack = append(stack, openBlock{keyword: keyword, line: line})
			case keyword == "end":
				// `{{ end }}` always closes the innermost open block, so an
				// under-count here is the only failure mode worth naming.
				if len(stack) == 0 {
					violations = append(violations, TemplateViolation{
						File: path, Line: line, Rule: "orphan-end",
						Message: "`{{ end }}` has no matching block opener — a guard or `define` lost its `{{- if }}`/`{{- define }}` when the file was split or moved",
					})
					continue
				}
				stack = stack[:len(stack)-1]
			}
			i = end + 1
		}
	}

	for _, open := range stack {
		violations = append(violations, TemplateViolation{
			File: path, Line: open.line, Rule: "unclosed",
			Message: fmt.Sprintf("`{{ %s }}` is never closed by `{{ end }}`", open.keyword),
		})
	}
	return violations
}

// findActionEnd returns the index of the `}}` closing the action that starts at
// from, or -1 when the action never closes. It skips over quoted strings so a
// `}}` inside a string literal does not terminate the action early.
func findActionEnd(raw []byte, from int) int {
	var quote byte
	for i := from; i < len(raw); i++ {
		c := raw[i]
		switch {
		case quote != 0:
			if c == '\\' {
				i++
				continue
			}
			if c == quote {
				quote = 0
			}
		case c == '"' || c == '`':
			quote = c
		case c == '}' && i+1 < len(raw) && raw[i+1] == '}':
			return i
		}
	}
	return -1
}

// actionKeyword returns the first bare word of a template action, with the
// trim markers (`{{-`, `-}}`) and any trailing punctuation removed. It returns
// "" for a pure expression or a comment.
func actionKeyword(action string) string {
	action = strings.TrimSpace(action)
	action = strings.TrimPrefix(action, "-")
	action = strings.TrimSuffix(action, "-")
	action = strings.TrimSpace(action)
	if action == "" || strings.HasPrefix(action, "/*") {
		return ""
	}
	word := action
	if idx := strings.IndexAny(word, " \t\n"); idx >= 0 {
		word = word[:idx]
	}
	return word
}

// templateFiles returns every Helm template file in a chart, walking both the
// umbrella templates/ dir and each subchart under charts/. Results are sorted so
// CI output is stable.
func templateFiles(chartDir string) ([]string, error) {
	var files []string

	roots := []string{filepath.Join(chartDir, "templates")}
	entries, err := os.ReadDir(filepath.Join(chartDir, "charts"))
	if err == nil {
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			roots = append(roots, filepath.Join(chartDir, "charts", entry.Name(), "templates"))
		}
	}

	seen := map[string]bool{}
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				// A chart with no templates/ (or no charts/ yet) is not an error.
				if os.IsNotExist(err) {
					return nil
				}
				return err
			}
			if d.IsDir() || !templateExtensions[strings.ToLower(filepath.Ext(path))] {
				return nil
			}
			rel, relErr := filepath.Rel(chartDir, path)
			if relErr != nil || seen[rel] {
				return nil
			}
			seen[rel] = true
			files = append(files, path)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	sort.Strings(files)
	return files, nil
}

// ExecuteTemplateBalance is the runner for `gitopsctl chart templates`: print
// one FAIL line per violation, exit 0 when clean and 1 when any violation.
func ExecuteTemplateBalance(chartDir string) int {
	violations, count := CheckTemplateBalance(chartDir)
	fmt.Printf("== Helm template balance: %d file(s) under %s\n", count, chartDir)

	for _, violation := range violations {
		fmt.Printf("  %s\n", violation)
	}
	if len(violations) > 0 {
		fmt.Printf("\n%d violation(s) — template block structure is NOT renderable\n", len(violations))
		return 1
	}
	fmt.Println("OK — every `if`/`range`/`with`/`define`/`block` has its `end`")
	return 0
}
