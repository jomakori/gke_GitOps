package classification

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type TestCase struct {
	ID          string   `yaml:"test_id"`
	Description string   `yaml:"description"`
	Input       string   `yaml:"input"`
	Expected    Expected `yaml:"expected"`
	Notes       string   `yaml:"notes"`
}

type Expected struct {
	Tier         string `yaml:"tier"`
	Domain       string `yaml:"domain"`
	PrimaryAgent string `yaml:"primary_agent"`
	LoopDepth    string `yaml:"loop_depth"`
	Verification string `yaml:"verification"`
	Gates        Gates  `yaml:"gates"`
}

type Gates struct {
	AskUser             bool `yaml:"ask_user"`
	RequireConfirmation bool `yaml:"require_confirmation"`
}

type ClassificationResult struct {
	Tier                string  `json:"tier"`
	Domain              string  `json:"domain"`
	PrimaryAgent        string  `json:"primary_agent"`
	LoopDepth           string  `json:"loop_depth"`
	Verification        string  `json:"verification"`
	AskUser             bool    `json:"ask_user"`
	RequireConfirmation bool    `json:"require_confirmation"`
	Confidence          float64 `json:"confidence"`
}

func ParseTestFile(path string) ([]TestCase, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	var tests []TestCase
	lines := strings.Split(string(data), "\n")
	var inBlock bool
	var blockLines []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if trimmed == "```yaml" {
			inBlock = true
			blockLines = nil
			continue
		}

		if trimmed == "```" && inBlock {
			inBlock = false
			var tc TestCase
			if err := yaml.Unmarshal([]byte(strings.Join(blockLines, "\n")), &tc); err != nil {
				return nil, fmt.Errorf("parse test block: %w", err)
			}
			if tc.ID != "" {
				tests = append(tests, tc)
			}
			continue
		}

		if inBlock {
			blockLines = append(blockLines, line)
		}
	}

	return tests, nil
}

func (tc *TestCase) Validate(result ClassificationResult) []string {
	var failures []string

	if result.Tier != tc.Expected.Tier {
		failures = append(failures,
			fmt.Sprintf("tier: got %q, want %q", result.Tier, tc.Expected.Tier))
	}

	if result.Domain != tc.Expected.Domain {
		failures = append(failures,
			fmt.Sprintf("domain: got %q, want %q", result.Domain, tc.Expected.Domain))
	}

	if result.PrimaryAgent != tc.Expected.PrimaryAgent {
		failures = append(failures,
			fmt.Sprintf("primary_agent: got %q, want %q", result.PrimaryAgent, tc.Expected.PrimaryAgent))
	}

	if result.Verification != tc.Expected.Verification {
		failures = append(failures,
			fmt.Sprintf("verification: got %q, want %q", result.Verification, tc.Expected.Verification))
	}

	if result.AskUser != tc.Expected.Gates.AskUser {
		failures = append(failures,
			fmt.Sprintf("ask_user: got %v, want %v", result.AskUser, tc.Expected.Gates.AskUser))
	}

	if result.RequireConfirmation != tc.Expected.Gates.RequireConfirmation {
		failures = append(failures,
			fmt.Sprintf("require_confirmation: got %v, want %v",
				result.RequireConfirmation, tc.Expected.Gates.RequireConfirmation))
	}

	return failures
}

func (tc *TestCase) Summary() string {
	return fmt.Sprintf("%s: %s (tier=%s, domain=%s, agent=%s)",
		tc.ID, tc.Description, tc.Expected.Tier, tc.Expected.Domain, tc.Expected.PrimaryAgent)
}
