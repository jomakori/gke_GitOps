package chart

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeChart(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", name, err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	return dir
}

func rulesFor(violations []TemplateViolation) []string {
	var out []string
	for _, v := range violations {
		out = append(out, v.Rule)
	}
	return out
}

func assertViolations(t *testing.T, files map[string]string, wantRules ...string) []TemplateViolation {
	t.Helper()
	got, _ := CheckTemplateBalance(writeChart(t, files))
	if len(got) != len(wantRules) {
		t.Fatalf("want %d violation(s) %v, got %d: %v", len(wantRules), wantRules, len(got), got)
	}
	for i, want := range wantRules {
		if got[i].Rule != want {
			t.Errorf("violation %d: want rule %q, got %q (%s)", i, want, got[i].Rule, got[i])
		}
	}
	return got
}

func TestCheckTemplateBalanceAcceptsBalancedTemplates(t *testing.T) {
	assertViolations(t, map[string]string{
		"templates/apps.yaml": "{{- if index .Values \"claude-proxy\" \"enabled\" }}\nkind: Service\n{{- end }}\n",
	})
}

func TestCheckTemplateBalanceAcceptsNestedBlocksWithElseIf(t *testing.T) {
	assertViolations(t, map[string]string{
		"templates/nested.yaml": strings.Join([]string{
			"{{- if .Values.a }}",
			"{{- range .Values.list }}",
			"{{- with .x }}",
			"{{- else if .y }}",
			"{{- end }}",
			"{{- end }}",
			"{{- end }}",
		}, "\n"),
	})
}

func TestCheckTemplateBalanceAcceptsDefinedAndBlockBlocks(t *testing.T) {
	assertViolations(t, map[string]string{
		"templates/_helpers.tpl": "{{- define \"app.fullname\" -}}\n{{- block \"app.label\" . }}\n{{- end }}\n{{- end }}\n",
		"templates/partials.tpl": "{{- define \"x\" }}{{ template \"y\" . }}{{ end }}",
	})
}

// An unbalanced guard split across two files is the exact regression this check
// was added for: the `{{- end }}` stays behind in the old file and the new file
// silently renders nothing.
func TestCheckTemplateBalanceFlagsOrphanEndFromSplitGuard(t *testing.T) {
	got := assertViolations(t, map[string]string{
		"templates/apps.yaml":                      "kind: ConfigMap\n{{- end }}\n",
		"templates/responses-proxy-configmap.yaml": "kind: ConfigMap\n",
	}, "orphan-end")

	if !strings.Contains(got[0].Message, "split or moved") {
		t.Errorf("orphan-end message should name the split/move cause, got %q", got[0].Message)
	}
}

func TestCheckTemplateBalanceFlagsUnclosedBlockAtOpeningLine(t *testing.T) {
	got := assertViolations(t, map[string]string{
		"templates/unclosed.yaml": "apiVersion: v1\nkind: Service\nmetadata:\n  name: b\n{{- if .Values.b }}\nspec: {}\n",
	}, "unclosed")

	if got[0].Line != 5 {
		t.Errorf("want unclosed reported at the opening line 5, got %d", got[0].Line)
	}
	if !strings.Contains(got[0].Message, "`{{ if }}`") {
		t.Errorf("unclosed message should name the unclosed keyword, got %q", got[0].Message)
	}
}

func TestCheckTemplateBalanceFlagsUnterminatedAction(t *testing.T) {
	assertViolations(t, map[string]string{
		"templates/bad.yaml": "a: {{ .Values.g\n",
	}, "unterminated")
}

// A literal `{{` in embedded prose, or `}}` inside a quoted string, must not be
// counted as block structure — the chart escapes prose braces via lit.Brace, and
// a false positive here would block every unrelated PR.
func TestCheckTemplateBalanceIgnoresProseAndStringLiterals(t *testing.T) {
	assertViolations(t, map[string]string{
		"templates/prose.yaml": strings.Join([]string{
			"note: |",
			"  a literal {{ brace and an action {{ .Values.x }} coexist",
			"s: {{ printf \"}}\" }}",
			"{{- if .Values.e }}",
			"ok",
			"{{- end }}",
		}, "\n"),
	})
}

func TestCheckTemplateBalanceIgnoresComments(t *testing.T) {
	assertViolations(t, map[string]string{
		"templates/commented.yaml": "{{/* a comment mentioning if and end */}}\n{{- if .Values.a }}\nok\n{{- end }}\n",
	})
}

func TestCheckTemplateBalanceWalksSubcharts(t *testing.T) {
	got := assertViolations(t, map[string]string{
		"templates/ok.yaml":                 "{{- if .Values.a }}\n{{- end }}\n",
		"charts/sub/templates/sub-bad.yaml": "{{- if .Values.b }}\nx\n",
	}, "unclosed")

	if !strings.Contains(got[0].File, filepath.Join("charts", "sub")) {
		t.Errorf("want the subchart file to be walked, got %q", got[0].File)
	}
}

// Every defect in one file is reported, not just the first: a developer who split
// a template should not have to re-run the check five times.
func TestCheckTemplateBalanceReportsAllViolationsPerFile(t *testing.T) {
	assertViolations(t, map[string]string{
		"templates/multi.yaml": strings.Join([]string{
			"{{- end }}",          // orphan
			"{{- if .Values.a }}", // unclosed
		}, "\n"),
	}, "orphan-end", "unclosed")
}

// Helm renders templates/*.yaml, *.yml, *.tpl and NOTES.txt; nothing else in the
// chart, and nothing outside templates/ at all, is template source. Chart.yaml
// and values.yaml sit at the chart root and must never be scanned.
func TestCheckTemplateBalanceIgnoresNonTemplateFiles(t *testing.T) {
	assertViolations(t, map[string]string{
		"Chart.yaml":         "name: x\nnote: {{ if }}\n",
		"values.yaml":        "a: {{ if }}\n",
		"README.md":          "{{ if }}\n",
		"templates/notes.md": "{{ if }}\n",
		"templates/ok.yaml":  "{{- if .Values.a }}\n{{- end }}\n",
	})
}

func TestCheckTemplateBalanceAcceptsChartWithoutTemplatesDir(t *testing.T) {
	violations, count := CheckTemplateBalance(writeChart(t, map[string]string{"Chart.yaml": "name: x\n"}))
	if len(violations) != 0 {
		t.Errorf("want no violations for a chart with no templates/, got %v", violations)
	}
	if count != 0 {
		t.Errorf("want 0 template files counted, got %d", count)
	}
}

func TestTemplateViolationStringCarriesFileLineAndRule(t *testing.T) {
	got := TemplateViolation{
		File: "templates/apps.yaml", Line: 102, Rule: "orphan-end", Message: "boom",
	}.String()

	for _, want := range []string{"FAIL", "templates/apps.yaml", ":102", "[orphan-end]", "boom"} {
		if !strings.Contains(got, want) {
			t.Errorf("String() output %q missing %q", got, want)
		}
	}
}

func TestExecuteTemplateBalanceExitCodes(t *testing.T) {
	clean := writeChart(t, map[string]string{"templates/ok.yaml": "{{- if .a }}\n{{- end }}\n"})
	if code := ExecuteTemplateBalance(clean); code != 0 {
		t.Errorf("want exit 0 for a balanced chart, got %d", code)
	}

	dirty := writeChart(t, map[string]string{"templates/bad.yaml": "{{- if .a }}\n"})
	if code := ExecuteTemplateBalance(dirty); code != 1 {
		t.Errorf("want exit 1 for an unbalanced chart, got %d", code)
	}
}
