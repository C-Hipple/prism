package llm

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/fatih/color"
)

const (
	ClaudeSonnetModel = "claude-sonnet-4-5-20250929"
	ClaudeHaikuModel  = "claude-3-5-haiku-latest"
	ClaudeMaxTokens   = 8000
)

// ClaudeClient is a wrapper for the Anthropic Claude API client.
type ClaudeClient struct {
	client  *anthropic.Client
	useFast bool
	verbose bool
}

// NewClaudeClient creates a new Claude API client.
func NewClaudeClient(apiKey string, useFast bool, verbose bool) *ClaudeClient {
	if apiKey == "" {
		color.Red("Warning: Claude API key is empty!")
	} else if verbose {
		color.Yellow("Claude API key length: %d characters", len(apiKey))
	}

	client := anthropic.NewClient(option.WithAPIKey(apiKey))

	if useFast {
		color.White("Using %s model.", ClaudeHaikuModel)
	} else {
		color.White("Using %s model.", ClaudeSonnetModel)
	}

	return &ClaudeClient{
		client:  &client,
		useFast: useFast,
		verbose: verbose,
	}
}

// ValidateAPIKey performs a minimal API call to verify the key is valid
func (c *ClaudeClient) ValidateAPIKey() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	message, err := c.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(ClaudeHaikuModel), // Use fast model
		MaxTokens: 10,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("test")),
		},
	})
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "API key") || strings.Contains(errMsg, "401") ||
			strings.Contains(errMsg, "403") || strings.Contains(errMsg, "authentication") ||
			strings.Contains(errMsg, "unauthorized") {
			return fmt.Errorf("ANTHROPIC_API_KEY validation failed - your API key is invalid or expired: %w", err)
		}
		return fmt.Errorf("Claude API validation failed: %w", err)
	}

	if len(message.Content) == 0 {
		return fmt.Errorf("Claude API validation failed: received empty response")
	}

	return nil
}

// GetReview sends the prompt to the Claude API and returns the review with token counts.
func (c *ClaudeClient) GetReview(prompt string) (string, int32, int32, int32, error) {
	ctx := context.Background()

	model := anthropic.Model(ClaudeSonnetModel)
	if c.useFast {
		model = anthropic.Model(ClaudeHaikuModel)
	}

	if c.verbose {
		color.Yellow("Sending request to Claude API...")
		color.Yellow("Model: %s", model)
		color.Yellow("Prompt length: %d characters", len(prompt))
		color.Yellow("Max tokens: %d", ClaudeMaxTokens)
	}

	message, err := c.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     model,
		MaxTokens: ClaudeMaxTokens,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(prompt)),
		},
	})
	if err != nil {
		// Check if error is related to authentication/invalid API key
		errMsg := err.Error()
		if strings.Contains(errMsg, "API key") || strings.Contains(errMsg, "401") || strings.Contains(errMsg, "403") || strings.Contains(errMsg, "authentication") || strings.Contains(errMsg, "unauthorized") {
			return "", 0, 0, 0, fmt.Errorf("Claude API authentication failed - your ANTHROPIC_API_KEY may be invalid or expired: %w", err)
		}
		return "", 0, 0, 0, fmt.Errorf("Claude API request failed: %w", err)
	}

	if c.verbose {
		color.Yellow("Received response from Claude API")
		color.Yellow("Response ID: %s", message.ID)
		color.Yellow("Model used: %s", message.Model)
		color.Yellow("Stop reason: %s", message.StopReason)
	}

	// Extract token usage from Claude API response
	var promptTokens, candidatesTokens, totalTokens int32
	promptTokens = int32(message.Usage.InputTokens)
	candidatesTokens = int32(message.Usage.OutputTokens)
	totalTokens = promptTokens + candidatesTokens

	if c.verbose {
		color.Yellow("Token usage - Input: %d, Output: %d, Total: %d", promptTokens, candidatesTokens, totalTokens)
	}

	if c.verbose {
		color.Yellow("Claude API response - content blocks: %d", len(message.Content))
	}

	if len(message.Content) == 0 {
		return "", 0, 0, 0, fmt.Errorf("no response from Claude API")
	}

	// Extract text from the response
	var reviewContent string
	for i, block := range message.Content {
		if c.verbose {
			color.Yellow("Processing content block %d of type: %T", i, block.AsAny())
		}

		// Try type switch for better debugging
		switch contentBlock := block.AsAny().(type) {
		case anthropic.TextBlock:
			if c.verbose {
				color.Yellow("Found TextBlock with %d characters", len(contentBlock.Text))
			}
			reviewContent += contentBlock.Text
		default:
			if c.verbose {
				color.Yellow("Skipping non-text content block of type: %T", contentBlock)
			}
		}
	}

	if reviewContent == "" {
		return "", 0, 0, 0, fmt.Errorf("unexpected response format from Claude API: no text content found in %d blocks", len(message.Content))
	}

	if c.verbose {
		color.Green("Successfully extracted %d characters from Claude response", len(reviewContent))
	}

	return reviewContent, promptTokens, candidatesTokens, totalTokens, nil
}

// GetReviewStream sends the prompt to the Claude API and streams the response with token counts.
func (c *ClaudeClient) GetReviewStream(prompt string, w io.Writer) (string, int32, int32, int32, error) {
	ctx := context.Background()

	model := anthropic.Model(ClaudeSonnetModel)
	if c.useFast {
		model = anthropic.Model(ClaudeHaikuModel)
	}

	if c.verbose {
		color.Yellow("Starting Claude streaming request...")
		color.Yellow("Model: %s", model)
		color.Yellow("Prompt length: %d characters", len(prompt))
	}

	stream := c.client.Messages.NewStreaming(ctx, anthropic.MessageNewParams{
		Model:     model,
		MaxTokens: ClaudeMaxTokens,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(prompt)),
		},
	})

	if c.verbose {
		color.Yellow("Stream created, starting to process events...")
	}

	var reviewContent string
	var eventCount int
	var promptTokens, candidatesTokens, totalTokens int32

	for stream.Next() {
		event := stream.Current()
		eventCount++

		if c.verbose {
			color.Yellow("Stream event #%d type: %T", eventCount, event.AsAny())
		}

		// Handle different event types from Claude's streaming API
		switch eventVariant := event.AsAny().(type) {
		case anthropic.MessageStartEvent:
			if c.verbose {
				color.Yellow("Message started")
			}
			// Extract token usage from message start event
			promptTokens = int32(eventVariant.Message.Usage.InputTokens)
			candidatesTokens = int32(eventVariant.Message.Usage.OutputTokens)
			if c.verbose {
				color.Yellow("Initial token usage - Input: %d, Output: %d", promptTokens, candidatesTokens)
			}
		case anthropic.ContentBlockStartEvent:
			if c.verbose {
				color.Yellow("Content block started")
			}
		case anthropic.ContentBlockDeltaEvent:
			// This is where the actual text content comes in
			switch deltaVariant := eventVariant.Delta.AsAny().(type) {
			case anthropic.TextDelta:
				chunk := deltaVariant.Text
				if c.verbose {
					color.Yellow("Received text chunk: %d characters", len(chunk))
				}
				reviewContent += chunk
				fmt.Fprint(w, chunk)
			default:
				if c.verbose {
					color.Yellow("Skipping non-text delta of type: %T", deltaVariant)
				}
			}
		case anthropic.ContentBlockStopEvent:
			if c.verbose {
				color.Yellow("Content block stopped")
			}
		case anthropic.MessageDeltaEvent:
			if c.verbose {
				color.Yellow("Message delta event")
			}
			// Update token usage from message delta
			candidatesTokens = int32(eventVariant.Usage.OutputTokens)
			if c.verbose {
				color.Yellow("Updated output tokens: %d", candidatesTokens)
			}
		case anthropic.MessageStopEvent:
			if c.verbose {
				color.Yellow("Message stopped")
			}
		default:
			if c.verbose {
				color.Yellow("Unknown event type: %T", eventVariant)
			}
		}
	}

	totalTokens = promptTokens + candidatesTokens

	if c.verbose {
		color.Yellow("Total stream events received: %d", eventCount)
		color.Yellow("Final token usage - Input: %d, Output: %d, Total: %d", promptTokens, candidatesTokens, totalTokens)
	}

	// Check for stream errors
	if err := stream.Err(); err != nil {
		color.Red("Stream error occurred: %v", err)
		// Check if error is related to authentication/invalid API key
		errMsg := err.Error()
		if strings.Contains(errMsg, "API key") || strings.Contains(errMsg, "401") || strings.Contains(errMsg, "403") || strings.Contains(errMsg, "authentication") || strings.Contains(errMsg, "unauthorized") {
			return "", 0, 0, 0, fmt.Errorf("Claude API authentication failed - your ANTHROPIC_API_KEY may be invalid or expired: %w", err)
		}
		return "", 0, 0, 0, fmt.Errorf("Claude API stream error: %w", err)
	}

	if c.verbose {
		color.Green("Stream complete: extracted %d characters total", len(reviewContent))
		if reviewContent == "" {
			modelName := ClaudeSonnetModel
			if c.useFast {
				modelName = ClaudeHaikuModel
			}
			color.Red("WARNING: Stream completed successfully but extracted 0 characters!")
			color.Red("This could indicate:")
			color.Red("  1. Invalid model name (check if '%s' is correct)", modelName)
			color.Red("  2. API key doesn't have access to this model")
			color.Red("  3. Request was filtered/blocked by safety systems")
			color.Red("  4. No ContentBlockDeltaEvent events were received")
		}
	}

	if reviewContent == "" {
		return "", 0, 0, 0, fmt.Errorf("stream completed but no text content was received (received %d events total)", eventCount)
	}

	return reviewContent, promptTokens, candidatesTokens, totalTokens, nil
}
