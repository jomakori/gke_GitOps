// Package litellm holds the repo-side policy for the openagent chart's LiteLLM
// proxy config — the Go half of the `validate litellm-config` command the
// gitopsctl README stages. Stdlib-only, so the scan is line-oriented: a key
// matters here purely by its indent relative to the enclosing litellm_params.
package litellm

import (
	"fmt"
	"os"
	"strings"
)

// Violation is one policy finding. The File/Line/Rule/Message shape and the FAIL
// line format mirror the chart and MCP lints so CI output stays greppable.
type Violation struct {
	File    string
	Line    int
	Rule    string
	Message string
}

func (v Violation) String() string {
	return fmt.Sprintf("FAIL %-46s :%-4d [%s] %s", v.File, v.Line, v.Rule, v.Message)
}

// cacheKeys must never appear inside a model's litellm_params. LiteLLM merges
// litellm_params into the completion kwargs, where the cache handler reads
// kwargs.get("cache", {}).get("no-cache", False) — a scalar raises
// AttributeError: 'bool' object has no attribute 'get' on every request, which
// the proxy reports as a 400 that looks like a provider fault. The supported
// spelling is litellm_settings.cache, which lands in litellm.cache instead.
var cacheKeys = []string{"cache", "cache_params"}

// CheckCacheKeysInModelParams reports every litellm_params block that sets
// cacheKeys. Scanned is the number of model_list entries examined.
func CheckCacheKeysInModelParams(path string) (violations []Violation, scanned int) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return []Violation{{File: path, Rule: "read", Message: err.Error()}}, 0
	}

	model := ""        // model_name of the entry being walked
	paramsIndent := -1 // indent of the open litellm_params block
	childIndent := -1  // indent of that block's direct keys, whatever the width

	for i, line := range strings.Split(string(raw), "\n") {
		indent, key, value, ok := parseKeyLine(line)
		if !ok {
			continue
		}

		if paramsIndent >= 0 {
			if indent <= paramsIndent {
				// A key at or left of the opening indent ends the block and is
				// then reconsidered as a possible new block opener.
				paramsIndent, childIndent = -1, -1
			} else {
				// Depth is tracked against the first key inside the block, so
				// only direct children are policy-relevant: a `cache` under some
				// other mapping (extra_body:) reaches the provider, not
				// kwargs["cache"], and an inner model_name must not overwrite
				// the entry name the message reports.
				if childIndent < 0 {
					childIndent = indent
				}
				if indent == childIndent {
					if key == cacheKeys[1] || (key == cacheKeys[0] && value != "") {
						violations = append(violations, cacheViolation(path, i+1, key, model, value))
					}
				}
				continue
			}
		}

		switch {
		case key == "model_name":
			model = unquote(value)

		case key == "litellm_params":
			scanned++
			if value != "" {
				if bad := flowCacheKeys(value); bad != "" {
					violations = append(violations, cacheViolation(path, i+1, bad, model, value))
				}
				continue
			}
			paramsIndent = indent
			childIndent = -1
			if model == "" {
				model = "?"
			}
		}
	}
	return violations, scanned
}

func cacheViolation(file string, line int, key, model, value string) Violation {
	return Violation{
		File: file,
		Line: line,
		Rule: "litellm-params-cache",
		Message: fmt.Sprintf("model %q: litellm_params sets %q=%s — LiteLLM merges this into the "+
			"request kwargs and reads it as a mapping, so every request 400s with "+
			"'bool' object has no attribute 'get'. Set litellm_settings.cache instead",
			model, key, strings.TrimSpace(value)),
	}
}

// flowCacheKeys returns the first cache key inside a one-line flow mapping, or "".
func flowCacheKeys(value string) string {
	for _, key := range cacheKeys {
		if strings.Contains(value, key+":") {
			return key
		}
	}
	return ""
}

// parseKeyLine splits a YAML line into indent, key and value, reporting ok=false
// for blank lines, comments, and non-mapping lines. Tabs are expanded to two
// spaces and the key is unquoted, so a reformat cannot smuggle a violation past
// a comparison against the literal key.
func parseKeyLine(line string) (indent int, key, value string, ok bool) {
	line = strings.ReplaceAll(line, "\t", "  ")
	trimmed := strings.TrimLeft(line, " ")
	indent = len(line) - len(trimmed)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return 0, "", "", false
	}
	switch {
	case strings.HasPrefix(trimmed, "- "):
		trimmed = strings.TrimLeft(trimmed[2:], " ")
		indent += 2
	case trimmed == "-":
		return 0, "", "", false
	}

	key, value, found := strings.Cut(trimmed, ":")
	key = unquote(strings.TrimSpace(key))
	if !found || key == "" {
		return 0, "", "", false
	}
	return indent, key, strings.TrimSpace(value), true
}

func unquote(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 {
		if q := s[0]; (q == '"' || q == '\'') && s[len(s)-1] == q {
			return s[1 : len(s)-1]
		}
	}
	return s
}

// Execute is the runner for `gitopsctl validate litellm-config`.
func Execute(path string) int {
	violations, scanned := CheckCacheKeysInModelParams(path)
	fmt.Printf("== LiteLLM model policy: %d model entr(ies) in %s\n", scanned, path)

	for _, violation := range violations {
		fmt.Printf("  %s\n", violation)
	}
	if len(violations) > 0 {
		fmt.Printf("\n%d violation(s) — a per-model cache key 400s every request to that route\n", len(violations))
		return 1
	}
	fmt.Println("OK — no per-model cache/cache_params; caching stays proxy-wide in litellm_settings")
	return 0
}
