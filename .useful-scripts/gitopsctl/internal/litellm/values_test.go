package litellm

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCleanValuesHaveNoViolations(t *testing.T) {
	violations, scanned := CheckCacheKeysInModelParams("testdata/clean.yaml")
	if len(violations) != 0 {
		t.Fatalf("clean fixture reported %d violation(s): %v", len(violations), violations)
	}
	if scanned != 3 {
		t.Errorf("scanned = %d, want 3 model entries", scanned)
	}
}

func TestCacheKeysInModelParamsAreReported(t *testing.T) {
	violations, scanned := CheckCacheKeysInModelParams("testdata/violations.yaml")
	if scanned != 5 {
		t.Errorf("scanned = %d, want 5 model entries", scanned)
	}
	if len(violations) != 3 {
		t.Fatalf("got %d violation(s), want 3: %v", len(violations), violations)
	}

	want := []struct{ model, key string }{
		{"deepseek-v4-flash-direct", "cache"},
		{"nested", "cache_params"},
		{"flowstyle", "cache"},
	}
	for i, w := range want {
		if !strings.Contains(violations[i].Message, `"`+w.model+`"`) {
			t.Errorf("violation %d does not name model %q: %s", i, w.model, violations[i].Message)
		}
		if !strings.Contains(violations[i].Message, `"`+w.key+`"`) {
			t.Errorf("violation %d does not name key %q: %s", i, w.key, violations[i].Message)
		}
		if violations[i].Rule != "litellm-params-cache" {
			t.Errorf("violation %d rule = %q", i, violations[i].Rule)
		}
	}

	// The last entry sets cache_read_input_token_cost, which starts with
	// "cache" but is a cost key, not the cache switch.
	last := violations[len(violations)-1]
	if strings.Contains(last.Message, "cache_read_input_token_cost") {
		t.Errorf("cost key mistaken for the cache switch: %s", last.Message)
	}
}

// The chart that shipped the incident must stay clean, so the exact regression
// is covered rather than only a synthetic fixture.
func TestOpenagentValuesAreClean(t *testing.T) {
	path := filepath.Join("..", "..", "..", "..", "services", "helm", "openagent", "values.yaml")
	if _, err := os.Stat(path); err != nil {
		t.Skipf("chart values not reachable from the module: %v", err)
	}
	violations, scanned := CheckCacheKeysInModelParams(path)
	if len(violations) != 0 {
		t.Fatalf("openagent values.yaml has %d violation(s): %v", len(violations), violations)
	}
	if scanned == 0 {
		t.Error("scanned 0 model entries — the walk found no model_list, so the check is inert")
	}
}

func TestMissingFileIsReportedNotIgnored(t *testing.T) {
	violations, _ := CheckCacheKeysInModelParams("testdata/does-not-exist.yaml")
	if len(violations) != 1 || violations[0].Rule != "read" {
		t.Fatalf("want one read violation, got %v", violations)
	}
}

func TestExecuteExitCodes(t *testing.T) {
	if got := Execute("testdata/clean.yaml"); got != 0 {
		t.Errorf("Execute(clean) = %d, want 0", got)
	}
	if got := Execute("testdata/violations.yaml"); got != 1 {
		t.Errorf("Execute(violations) = %d, want 1", got)
	}
}

func TestParseKeyLine(t *testing.T) {
	tests := []struct {
		line       string
		wantOK     bool
		wantKey    string
		wantValue  string
		wantIndent int
	}{
		{line: "          cache: true", wantOK: true, wantKey: "cache", wantValue: "true", wantIndent: 10},
		{line: "      - model_name: alpha", wantOK: true, wantKey: "model_name", wantValue: "alpha", wantIndent: 8},
		{line: "        litellm_params:", wantOK: true, wantKey: "litellm_params", wantIndent: 8},
		{line: "  # a comment", wantOK: false},
		{line: "", wantOK: false},
		{line: "   ", wantOK: false},
		{line: "      - ", wantOK: false},
		{line: "        just-a-scalar", wantOK: false},
	}
	for _, tt := range tests {
		indent, key, value, ok := parseKeyLine(tt.line)
		if ok != tt.wantOK {
			t.Errorf("parseKeyLine(%q) ok = %v, want %v", tt.line, ok, tt.wantOK)
			continue
		}
		if !ok {
			continue
		}
		if key != tt.wantKey || value != tt.wantValue || indent != tt.wantIndent {
			t.Errorf("parseKeyLine(%q) = (%d, %q, %q), want (%d, %q, %q)",
				tt.line, indent, key, value, tt.wantIndent, tt.wantKey, tt.wantValue)
		}
	}
}

func TestUnquote(t *testing.T) {
	for in, want := range map[string]string{
		`"quoted"`: "quoted",
		"'quoted'": "quoted",
		"plain":    "plain",
		`"`:        `"`,
		"":         "",
	} {
		if got := unquote(in); got != want {
			t.Errorf("unquote(%q) = %q, want %q", in, got, want)
		}
	}
}

// Each case is a shape an earlier revision of the scan either missed silently
// or reported against the wrong model name.
func TestCacheKeyShapes(t *testing.T) {
	tests := []struct {
		name       string
		yaml       string
		wantModel  string
		wantReport bool
	}{
		{
			name:       "quoted key is still the cache key",
			yaml:       "model_list:\n  - model_name: a\n    litellm_params:\n      \"cache\": true\n",
			wantModel:  "a",
			wantReport: true,
		},
		{
			name:       "tab indentation does not hide the key",
			yaml:       "model_list:\n\t- model_name: a\n\t  litellm_params:\n\t    cache: true\n",
			wantModel:  "a",
			wantReport: true,
		},
		{
			name:       "an inner model_name does not shadow the entry name",
			yaml:       "model_list:\n  - model_name: a\n    litellm_params:\n      model_name: inner\n      cache: true\n",
			wantModel:  "a",
			wantReport: true,
		},
		{
			name:       "a cache under another mapping is not the kwargs cache",
			yaml:       "model_list:\n  - model_name: a\n    litellm_params:\n      extra_body:\n        cache: true\n",
			wantReport: false,
		},
		{
			name:       "CRLF line endings",
			yaml:       "model_list:\r\n  - model_name: a\r\n    litellm_params:\r\n      cache: true\r\n",
			wantModel:  "a",
			wantReport: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "values.yaml")
			if err := os.WriteFile(path, []byte(tt.yaml), 0o600); err != nil {
				t.Fatal(err)
			}
			violations, _ := CheckCacheKeysInModelParams(path)
			if !tt.wantReport {
				if len(violations) != 0 {
					t.Fatalf("want no violation, got %v", violations)
				}
				return
			}
			if len(violations) != 1 {
				t.Fatalf("want 1 violation, got %d: %v", len(violations), violations)
			}
			if !strings.Contains(violations[0].Message, `"`+tt.wantModel+`"`) {
				t.Errorf("violation %q does not name model %q", violations[0].Message, tt.wantModel)
			}
		})
	}
}
