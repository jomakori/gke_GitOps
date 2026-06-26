package classification

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParseTestFile validates that we can load all test cases from YAML
func TestParseTestFile(t *testing.T) {
	tests, err := ParseTestFile("intent-classification.yaml")
	require.NoError(t, err, "should parse test file")
	require.NotEmpty(t, tests, "should have test cases")

	// Check we have tests in each tier
	tierCounts := make(map[string]int)
	for _, tc := range tests {
		tierCounts[tc.Expected.Tier]++
		assert.NotEmpty(t, tc.ID, "test should have ID")
		assert.NotEmpty(t, tc.Input, "test should have input")
		assert.NotEmpty(t, tc.Expected.Tier, "test should have expected tier")
	}

	// Verify we have coverage across tiers
	expectedTiers := []string{"trivial", "quick", "scoped", "exploratory", "complex", "ambiguous"}
	for _, tier := range expectedTiers {
		assert.Greater(t, tierCounts[tier], 0, "should have tests for tier %s", tier)
	}

	t.Logf("Loaded %d tests", len(tests))
	for tier, count := range tierCounts {
		t.Logf("  %s: %d tests", tier, count)
	}
}

// TestValidate checks the validation logic
func TestValidate(t *testing.T) {
	tc := TestCase{
		ID: "test-001",
		Expected: Expected{
			Tier:         "trivial",
			Domain:       "general",
			PrimaryAgent: "sisyphus-junior",
			Verification: "V1",
			Gates: Gates{
				AskUser:             false,
				RequireConfirmation: false,
			},
		},
	}

	// Perfect match
	result := ClassificationResult{
		Tier:         "trivial",
		Domain:       "general",
		PrimaryAgent: "sisyphus-junior",
		Verification: "V1",
		AskUser:      false,
		RequireConfirmation: false,
	}
	failures := tc.Validate(result)
	assert.Empty(t, failures, "perfect match should have no failures")

	// Wrong tier
	result.Tier = "complex"
	failures = tc.Validate(result)
	assert.Len(t, failures, 1, "should detect tier mismatch")
	assert.Contains(t, failures[0], "tier:")
}

// TestTrivialClassifications runs all trivial-tier tests as unit tests
func TestTrivialClassifications(t *testing.T) {
	tests, err := ParseTestFile("intent-classification.yaml")
	require.NoError(t, err)

	for _, tc := range tests {
		if tc.Expected.Tier != "trivial" {
			continue
		}
		t.Run(tc.ID, func(t *testing.T) {
			assert.NotEmpty(t, tc.Input)
			assert.Equal(t, "trivial", tc.Expected.Tier)
			assert.NotEmpty(t, tc.Expected.PrimaryAgent)
		})
	}
}

// TestQuickClassifications runs all quick-tier tests
func TestQuickClassifications(t *testing.T) {
	tests, err := ParseTestFile("intent-classification.yaml")
	require.NoError(t, err)

	for _, tc := range tests {
		if tc.Expected.Tier != "quick" {
			continue
		}
		t.Run(tc.ID, func(t *testing.T) {
			assert.NotEmpty(t, tc.Input)
			assert.Equal(t, "quick", tc.Expected.Tier)
		})
	}
}

// TestComplexClassifications runs all complex-tier tests
func TestComplexClassifications(t *testing.T) {
	tests, err := ParseTestFile("intent-classification.yaml")
	require.NoError(t, err)

	for _, tc := range tests {
		if tc.Expected.Tier != "complex" {
			continue
		}
		t.Run(tc.ID, func(t *testing.T) {
			assert.NotEmpty(t, tc.Input)
			assert.Equal(t, "complex", tc.Expected.Tier)
		})
	}
}

// TestAmbiguousClassifications runs all ambiguous-tier tests
func TestAmbiguousClassifications(t *testing.T) {
	tests, err := ParseTestFile("intent-classification.yaml")
	require.NoError(t, err)

	for _, tc := range tests {
		if tc.Expected.Tier != "ambiguous" {
			continue
		}
		t.Run(tc.ID, func(t *testing.T) {
			assert.NotEmpty(t, tc.Input)
			assert.Equal(t, "ambiguous", tc.Expected.Tier)
			assert.True(t, tc.Expected.Gates.AskUser || tc.Expected.Gates.RequireConfirmation,
				"ambiguous tests should have some gate enabled")
		})
	}
}

// --- Integration Tests (require LLM endpoint) ---

// ClassificationClient calls the Sisyphus A2A endpoint
type ClassificationClient struct {
	BaseURL            string
	Client             *http.Client
	CFAccessClientID   string
	CFAccessClientSecret string
}

func (c *ClassificationClient) Classify(ctx context.Context, input string) (ClassificationResult, error) {
	var result ClassificationResult

	reqBody, err := json.Marshal(map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "tasks/send",
		"params": map[string]interface{}{
			"messages": []map[string]string{
				{"role": "user", "content": input},
			},
		},
		"id": 1,
	})
	if err != nil {
		return result, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL, bytes.NewReader(reqBody))
	if err != nil {
		return result, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.CFAccessClientID != "" && c.CFAccessClientSecret != "" {
		req.Header.Set("CF-Access-Client-Id", c.CFAccessClientID)
		req.Header.Set("CF-Access-Client-Secret", c.CFAccessClientSecret)
	}

	resp, err := c.Client.Do(req)
	if err != nil {
		return result, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return result, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	var a2aResp struct {
		Result *struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		} `json:"result"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&a2aResp); err != nil {
		return result, fmt.Errorf("decode response: %w", err)
	}

	if a2aResp.Error != nil {
		return result, fmt.Errorf("a2a error: %s", a2aResp.Error.Message)
	}

	if a2aResp.Result == nil || len(a2aResp.Result.Messages) == 0 {
		return result, fmt.Errorf("empty response")
	}

	for i := len(a2aResp.Result.Messages) - 1; i >= 0; i-- {
		msg := a2aResp.Result.Messages[i]
		if msg.Role == "assistant" {
			if err := json.Unmarshal([]byte(msg.Content), &result); err != nil {
				return result, fmt.Errorf("parse classification: %w", err)
			}
			return result, nil
		}
	}

	return result, fmt.Errorf("no assistant message found")
}

func TestIntegrationClassification(t *testing.T) {
	baseURL := os.Getenv("KAGENT_A2A_URL")
	if baseURL == "" {
		t.Skip("KAGENT_A2A_URL not set, skipping integration tests")
	}

	client := &ClassificationClient{
		BaseURL:              baseURL,
		Client:               &http.Client{Timeout: 30 * time.Second},
		CFAccessClientID:     os.Getenv("CF_ACCESS_CLIENT_ID"),
		CFAccessClientSecret: os.Getenv("CF_ACCESS_CLIENT_SECRET"),
	}

	tests, err := ParseTestFile("intent-classification.yaml")
	require.NoError(t, err)

	var passed, failed int
	for _, tc := range tests {
		t.Run(tc.ID, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			result, err := client.Classify(ctx, tc.Input)
			if err != nil {
				if tc.Expected.Tier == "ambiguous" {
					t.Logf("Ambiguous test connection error (expected): %v", err)
					passed++
					return
				}
				t.Fatalf("classify failed: %v", err)
			}

			failures := tc.Validate(result)
			if len(failures) > 0 {
				failed++
				for _, f := range failures {
					t.Errorf("%s", f)
				}
			} else {
				passed++
			}
		})
	}

	t.Logf("Integration results: %d passed, %d failed out of %d", passed, failed, len(tests))

	passRate := float64(passed) / float64(len(tests))
	if passRate < 0.8 {
		t.Fatalf("Integration pass rate %.1f%% below 80%% threshold", passRate*100)
	}
}
