package strix

import (
	"testing"

	"go.uber.org/zap"
)

func TestClientInitialization(t *testing.T) {
	logger := zap.NewNop()

	client := NewClient(
		"strix",
		"./strix_runs",
		"openai/gpt-4o",
		"test-key",
		logger,
	)

	if client == nil {
		t.Fatal("NewClient returned nil")
	}

	if client.strixPath != "strix" {
		t.Errorf("expected strixPath 'strix', got %q", client.strixPath)
	}

	if client.runsDir != "./strix_runs" {
		t.Errorf("expected runsDir './strix_runs', got %q", client.runsDir)
	}

	if client.llmModel != "openai/gpt-4o" {
		t.Errorf("expected llmModel 'openai/gpt-4o', got %q", client.llmModel)
	}
}

func TestClientInitializationWithDefaults(t *testing.T) {
	logger := zap.NewNop()

	client := NewClient("", "", "openai/gpt-4o", "test-key", logger)

	if client == nil {
		t.Fatal("NewClient returned nil")
	}

	// Should use defaults
	if client.strixPath != "strix" {
		t.Errorf("expected default strixPath 'strix', got %q", client.strixPath)
	}

	if client.runsDir != "./strix_runs" {
		t.Errorf("expected default runsDir './strix_runs', got %q", client.runsDir)
	}
}

func TestExtractRunID(t *testing.T) {
	testCases := []struct {
		name     string
		output   string
		expected string
	}{
		{
			name:     "valid run id",
			output:   "Scan complete. Results saved to strix_runs/2024-01-15T10-30-45-abc123 successfully.",
			expected: "2024-01-15T10-30-45-abc123",
		},
		{
			name:     "run id in middle of output",
			output:   "Starting scan...\nScan complete. Results saved to strix_runs/test-run-id-xyz\nDone!",
			expected: "test-run-id-xyz",
		},
		{
			name:     "no run id",
			output:   "No run results found",
			expected: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := extractRunID(tc.output)
			if result != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, result)
			}
		})
	}
}
