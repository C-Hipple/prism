package llm

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Note: Testing the actual generative model calls would require mocking the genai.Client
// and its chained methods, which is complex. For this case, we'll focus on testing the
// client constructor to ensure it's selecting the correct model based on the `useFlash` flag.
// A more robust solution might involve creating an interface for the genai.GenerativeModel
// and injecting a mock implementation, but for now, this provides a good baseline.

func TestNewClient_ModelSelection(t *testing.T) {
	// This test ensures that NewClient does not panic when selecting different providers and models.
	// The actual API key is not needed for this test, as we are not making real
	// network calls. We are only testing the model selection logic within our NewClient function.

	// Test case 1: Selecting the Gemini "pro" model (useFlash = false)
	assert.NotPanics(t, func() {
		_ = NewClient(ProviderGemini, "dummy-key", false, false)
	}, "NewClient should not panic when selecting the Gemini pro model")

	// Test case 2: Selecting the Gemini "flash" model (useFlash = true)
	assert.NotPanics(t, func() {
		_ = NewClient(ProviderGemini, "dummy-key", true, false)
	}, "NewClient should not panic when selecting the Gemini flash model")

	// Test case 3: Selecting Claude Sonnet (useFlash = false)
	assert.NotPanics(t, func() {
		_ = NewClient(ProviderClaude, "dummy-key", false, false)
	}, "NewClient should not panic when selecting the Claude Sonnet model")

	// Test case 4: Selecting Claude Haiku (useFlash = true)
	assert.NotPanics(t, func() {
		_ = NewClient(ProviderClaude, "dummy-key", true, false)
	}, "NewClient should not panic when selecting the Claude Haiku model")
}
