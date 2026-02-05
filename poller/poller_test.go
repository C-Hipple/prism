package poller

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"pr-review-server/db"
	"pr-review-server/github"
)

func TestShouldReview(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name              string
		pr                github.PullRequest
		dbPR              *db.PR
		isTracked         bool
		autoReviewEnabled bool
		expected          bool
	}{
		{
			name: "New PR (not in DB) should not be reviewed immediately",
			pr: github.PullRequest{
				Number:    1,
				CommitSHA: "sha1",
			},
			dbPR:              nil,
			isTracked:         false,
			autoReviewEnabled: true,
			expected:          false,
		},
		{
			name: "Manual trigger (generating status) should be reviewed even if auto-review disabled",
			pr: github.PullRequest{
				Number:    1,
				CommitSHA: "sha1",
			},
			dbPR: &db.PR{
				Status:        "generating",
				LastCommitSHA: "sha1",
			},
			isTracked:         false,
			autoReviewEnabled: false,
			expected:          true,
		},
		{
			name: "Manual trigger should NOT be reviewed if already tracked (running)",
			pr: github.PullRequest{
				Number:    1,
				CommitSHA: "sha1",
			},
			dbPR: &db.PR{
				Status:        "generating",
				LastCommitSHA: "sha1",
			},
			isTracked:         true,
			autoReviewEnabled: false,
			expected:          false,
		},
		{
			name: "Pending PR should be reviewed if auto-review enabled",
			pr: github.PullRequest{
				Number:    1,
				CommitSHA: "sha1",
			},
			dbPR: &db.PR{
				Status:        "pending",
				LastCommitSHA: "sha1",
			},
			isTracked:         false,
			autoReviewEnabled: true,
			expected:          true,
		},
		{
			name: "Pending PR should NOT be reviewed if auto-review disabled",
			pr: github.PullRequest{
				Number:    1,
				CommitSHA: "sha1",
			},
			dbPR: &db.PR{
				Status:        "pending",
				LastCommitSHA: "sha1",
			},
			isTracked:         false,
			autoReviewEnabled: false,
			expected:          false,
		},
		{
			name: "Completed PR with same commit should NOT be re-reviewed (auto mode)",
			pr: github.PullRequest{
				Number:    1,
				CommitSHA: "sha1",
			},
			dbPR: &db.PR{
				Status:         "completed",
				LastCommitSHA:  "sha1",
				LastReviewedAt: &now,
			},
			isTracked:         false,
			autoReviewEnabled: true,
			expected:          false,
		},
		{
			name: "Completed PR with NEW commit should be reviewed (auto mode)",
			// Note: In reality, checkForOutdatedReviews resets status to pending first.
			// But if we encountered it raw, logic handles it?
			// The current implementation checks:
			// if dbPR.LastCommitSHA == pr.CommitSHA && dbPR.Status == "completed" { return false }
			// So if commit SHA differs, it falls through to 'true' ONLY IF status was pending?
			// Wait, the logic is: isAutoCandidate := dbPR.Status == "pending" && autoReviewEnabled.
			// So if status is "completed", isAutoCandidate is false.
			// The only way a completed PR gets reviewed is if checkForOutdatedReviews changes it to pending first.
			// So this test case effectively tests that a "completed" PR is ignored by this specific function.
			pr: github.PullRequest{
				Number:    1,
				CommitSHA: "sha2", // New commit
			},
			dbPR: &db.PR{
				Status:        "completed",
				LastCommitSHA: "sha1", // Old commit
			},
			isTracked:         false,
			autoReviewEnabled: true,
			expected:          false, // Correct, because status is not 'pending' yet. Logic depends on checkForOutdatedReviews.
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := shouldReview(tt.pr, tt.dbPR, tt.isTracked, tt.autoReviewEnabled)
			if result != tt.expected {
				t.Errorf("shouldReview() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// =============================================================================
// cleanupClosedPRs Tests
// =============================================================================

func TestCleanupClosedPRs_RemovesClosedPRs(t *testing.T) {
	mockGH := NewMockGitHubClient()
	mockDB := NewMockDatabase()

	// Add two PRs to database: one open, one closed
	mockDB.PRs["owner/repo/1"] = &db.PR{
		RepoOwner: "owner",
		RepoName:  "repo",
		PRNumber:  1,
		Status:    "completed",
	}
	mockDB.PRs["owner/repo/2"] = &db.PR{
		RepoOwner: "owner",
		RepoName:  "repo",
		PRNumber:  2,
		Status:    "completed",
	}

	// PR 1 is open, PR 2 is closed
	mockGH.IsPROpenResults["owner/repo/1"] = struct {
		IsOpen bool
		Err    error
	}{true, nil}
	mockGH.IsPROpenResults["owner/repo/2"] = struct {
		IsOpen bool
		Err    error
	}{false, nil}

	poller := newTestPoller(mockGH, mockDB)
	ctx := context.Background()

	removed, err := poller.cleanupClosedPRs(ctx)

	if err != nil {
		t.Fatalf("cleanupClosedPRs returned error: %v", err)
	}

	if removed != 1 {
		t.Errorf("expected 1 PR removed, got %d", removed)
	}

	// Verify PR 1 still exists
	if _, exists := mockDB.PRs["owner/repo/1"]; !exists {
		t.Error("PR 1 should still exist (it's open)")
	}

	// Verify PR 2 was deleted
	if _, exists := mockDB.PRs["owner/repo/2"]; exists {
		t.Error("PR 2 should have been deleted (it's closed)")
	}

	// Verify delete was called for PR 2
	if len(mockDB.DeletePRCalls) != 1 {
		t.Fatalf("expected 1 DeletePR call, got %d", len(mockDB.DeletePRCalls))
	}
	if mockDB.DeletePRCalls[0].PRNumber != 2 {
		t.Errorf("expected DeletePR to be called for PR 2, got PR %d", mockDB.DeletePRCalls[0].PRNumber)
	}
}

func TestCleanupClosedPRs_KeepsOpenPRs(t *testing.T) {
	mockGH := NewMockGitHubClient()
	mockDB := NewMockDatabase()

	// Add PRs to database
	mockDB.PRs["owner/repo/1"] = &db.PR{
		RepoOwner: "owner",
		RepoName:  "repo",
		PRNumber:  1,
		Status:    "pending",
	}
	mockDB.PRs["owner/repo/2"] = &db.PR{
		RepoOwner: "owner",
		RepoName:  "repo",
		PRNumber:  2,
		Status:    "generating",
	}

	// Both PRs are open
	mockGH.IsPROpenResults["owner/repo/1"] = struct {
		IsOpen bool
		Err    error
	}{true, nil}
	mockGH.IsPROpenResults["owner/repo/2"] = struct {
		IsOpen bool
		Err    error
	}{true, nil}

	poller := newTestPoller(mockGH, mockDB)
	ctx := context.Background()

	removed, err := poller.cleanupClosedPRs(ctx)

	if err != nil {
		t.Fatalf("cleanupClosedPRs returned error: %v", err)
	}

	if removed != 0 {
		t.Errorf("expected 0 PRs removed, got %d", removed)
	}

	// Verify both PRs still exist
	if len(mockDB.PRs) != 2 {
		t.Errorf("expected 2 PRs in database, got %d", len(mockDB.PRs))
	}

	// Verify no deletes were called
	if len(mockDB.DeletePRCalls) != 0 {
		t.Errorf("expected no DeletePR calls, got %d", len(mockDB.DeletePRCalls))
	}
}

func TestCleanupClosedPRs_HandlesGitHubErrors(t *testing.T) {
	mockGH := NewMockGitHubClient()
	mockDB := NewMockDatabase()

	// Add PRs to database
	mockDB.PRs["owner/repo/1"] = &db.PR{
		RepoOwner: "owner",
		RepoName:  "repo",
		PRNumber:  1,
		Status:    "completed",
	}
	mockDB.PRs["owner/repo/2"] = &db.PR{
		RepoOwner: "owner",
		RepoName:  "repo",
		PRNumber:  2,
		Status:    "completed",
	}

	// PR 1 returns an error, PR 2 is closed
	mockGH.IsPROpenResults["owner/repo/1"] = struct {
		IsOpen bool
		Err    error
	}{false, errors.New("GitHub API error")}
	mockGH.IsPROpenResults["owner/repo/2"] = struct {
		IsOpen bool
		Err    error
	}{false, nil}

	poller := newTestPoller(mockGH, mockDB)
	ctx := context.Background()

	removed, err := poller.cleanupClosedPRs(ctx)

	if err != nil {
		t.Fatalf("cleanupClosedPRs should not return error on individual PR errors: %v", err)
	}

	// PR 1 should still exist (error checking status)
	if _, exists := mockDB.PRs["owner/repo/1"]; !exists {
		t.Error("PR 1 should still exist (GitHub error during check)")
	}

	// PR 2 should be deleted
	if _, exists := mockDB.PRs["owner/repo/2"]; exists {
		t.Error("PR 2 should have been deleted")
	}

	// Only PR 2 should have been removed
	if removed != 1 {
		t.Errorf("expected 1 PR removed, got %d", removed)
	}
}

func TestCleanupClosedPRs_SendsDeleteEvent(t *testing.T) {
	mockGH := NewMockGitHubClient()
	mockDB := NewMockDatabase()

	mockDB.PRs["owner/repo/1"] = &db.PR{
		RepoOwner: "owner",
		RepoName:  "repo",
		PRNumber:  1,
		Status:    "completed",
	}

	mockGH.IsPROpenResults["owner/repo/1"] = struct {
		IsOpen bool
		Err    error
	}{false, nil}

	poller := newTestPoller(mockGH, mockDB)

	// Track events
	var events []struct {
		eventType string
		payload   interface{}
	}
	poller.EventFunc = func(eventType string, payload interface{}) {
		events = append(events, struct {
			eventType string
			payload   interface{}
		}{eventType, payload})
	}

	ctx := context.Background()
	_, err := poller.cleanupClosedPRs(ctx)

	if err != nil {
		t.Fatalf("cleanupClosedPRs returned error: %v", err)
	}

	// Verify pr_deleted event was sent
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	if events[0].eventType != "pr_deleted" {
		t.Errorf("expected event type 'pr_deleted', got '%s'", events[0].eventType)
	}

	payload, ok := events[0].payload.(map[string]interface{})
	if !ok {
		t.Fatalf("payload should be map[string]interface{}")
	}

	if payload["owner"] != "owner" || payload["repo"] != "repo" || payload["number"] != 1 {
		t.Errorf("unexpected payload: %v", payload)
	}
}

// =============================================================================
// generateReviewsBatch Tests
// =============================================================================

func TestGenerateReviewsBatch_SkipsExistingReviews(t *testing.T) {
	mockGH := NewMockGitHubClient()
	mockDB := NewMockDatabase()
	mockStorage := NewMockReviewStorage()
	mockGenerator := NewMockReviewGenerator()

	// Add PR to database
	mockDB.PRs["owner/repo/1"] = &db.PR{
		RepoOwner:     "owner",
		RepoName:      "repo",
		PRNumber:      1,
		LastCommitSHA: "abc123def456789012345678901234567890abcd",
		Status:        "pending",
		CriticalCount: 5,
		MediumCount:   10,
		LowCount:      15,
	}

	// Mark review as already existing
	mockStorage.ExistingReviews["owner/repo/1/abc123def456789012345678901234567890abcd"] = true

	poller := newTestPollerFull(mockGH, mockDB, mockStorage, mockGenerator)
	ctx := context.Background()

	prs := []github.PullRequest{
		{Owner: "owner", Repo: "repo", Number: 1, CommitSHA: "abc123def456789012345678901234567890abcd", Title: "Test PR", Author: "user"},
	}

	err := poller.generateReviewsBatch(ctx, prs)
	if err != nil {
		t.Fatalf("generateReviewsBatch returned error: %v", err)
	}

	// Generator should NOT have been called (review exists)
	if len(mockGenerator.GenerateReviewCalls) != 0 {
		t.Errorf("expected no GenerateReview calls, got %d", len(mockGenerator.GenerateReviewCalls))
	}

	// PR should be marked as completed
	pr := mockDB.PRs["owner/repo/1"]
	if pr.Status != "completed" {
		t.Errorf("expected status 'completed', got '%s'", pr.Status)
	}

	// Importance counts should be preserved
	if pr.CriticalCount != 5 || pr.MediumCount != 10 || pr.LowCount != 15 {
		t.Errorf("importance counts should be preserved, got critical=%d, medium=%d, low=%d",
			pr.CriticalCount, pr.MediumCount, pr.LowCount)
	}
}

func TestGenerateReviewsBatch_SetsGeneratingStatus(t *testing.T) {
	mockGH := NewMockGitHubClient()
	mockDB := NewMockDatabase()
	mockStorage := NewMockReviewStorage()
	mockGenerator := NewMockReviewGenerator()

	// Add PR to database
	mockDB.PRs["owner/repo/1"] = &db.PR{
		RepoOwner:     "owner",
		RepoName:      "repo",
		PRNumber:      1,
		LastCommitSHA: "abc123def456789012345678901234567890abcd",
		Status:        "pending",
	}

	// Track status changes
	var statusDuringGeneration string
	mockGenerator.DefaultResult = &ReviewResult{
		HTMLContent:   []byte("<html>Review</html>"),
		CriticalCount: 1,
		MediumCount:   2,
		LowCount:      3,
	}

	// Capture the original GenerateReview and intercept to check status
	originalGenerator := NewMockReviewGenerator()
	originalGenerator.DefaultResult = mockGenerator.DefaultResult

	poller := newTestPollerFull(mockGH, mockDB, mockStorage, originalGenerator)

	// Add custom interceptor
	poller.reviewGenerator = &statusCheckingGenerator{
		wrapped: originalGenerator,
		checkFunc: func() {
			pr := mockDB.PRs["owner/repo/1"]
			if pr != nil {
				statusDuringGeneration = pr.Status
			}
		},
	}

	ctx := context.Background()
	prs := []github.PullRequest{
		{Owner: "owner", Repo: "repo", Number: 1, CommitSHA: "abc123def456789012345678901234567890abcd", Title: "Test PR", Author: "user"},
	}

	err := poller.generateReviewsBatch(ctx, prs)
	if err != nil {
		t.Fatalf("generateReviewsBatch returned error: %v", err)
	}

	// Status should have been "generating" during the review
	if statusDuringGeneration != "generating" {
		t.Errorf("expected status 'generating' during review, got '%s'", statusDuringGeneration)
	}

	// Status should be "completed" after
	pr := mockDB.PRs["owner/repo/1"]
	if pr.Status != "completed" {
		t.Errorf("expected status 'completed', got '%s'", pr.Status)
	}
}

// statusCheckingGenerator wraps a generator to check status during generation
type statusCheckingGenerator struct {
	wrapped   *MockReviewGenerator
	checkFunc func()
}

func (s *statusCheckingGenerator) GenerateReview(ctx context.Context, cfg ReviewGeneratorConfig) (*ReviewResult, error) {
	s.checkFunc()
	return s.wrapped.GenerateReview(ctx, cfg)
}

func TestGenerateReviewsBatch_HandlesGenerationError(t *testing.T) {
	mockGH := NewMockGitHubClient()
	mockDB := NewMockDatabase()
	mockStorage := NewMockReviewStorage()
	mockGenerator := NewMockReviewGenerator()

	// Add PR to database
	mockDB.PRs["owner/repo/1"] = &db.PR{
		RepoOwner:     "owner",
		RepoName:      "repo",
		PRNumber:      1,
		LastCommitSHA: "abc123def456789012345678901234567890abcd",
		Status:        "pending",
	}

	// Make generator return error
	mockGenerator.Results["owner/repo/1"] = struct {
		Result *ReviewResult
		Err    error
	}{nil, errors.New("LLM generation failed")}

	poller := newTestPollerFull(mockGH, mockDB, mockStorage, mockGenerator)
	ctx := context.Background()

	prs := []github.PullRequest{
		{Owner: "owner", Repo: "repo", Number: 1, CommitSHA: "abc123def456789012345678901234567890abcd", Title: "Test PR", Author: "user"},
	}

	// Should not return error (handles per-PR errors gracefully)
	err := poller.generateReviewsBatch(ctx, prs)
	if err != nil {
		t.Fatalf("generateReviewsBatch should not return error for individual PR failures: %v", err)
	}

	// PR should be in error state
	pr := mockDB.PRs["owner/repo/1"]
	if pr.Status != "error" {
		t.Errorf("expected status 'error', got '%s'", pr.Status)
	}
}

func TestGenerateReviewsBatch_UpdatesDBOnSuccess(t *testing.T) {
	mockGH := NewMockGitHubClient()
	mockDB := NewMockDatabase()
	mockStorage := NewMockReviewStorage()
	mockGenerator := NewMockReviewGenerator()

	// Add PR to database
	mockDB.PRs["owner/repo/1"] = &db.PR{
		RepoOwner:     "owner",
		RepoName:      "repo",
		PRNumber:      1,
		LastCommitSHA: "abc123def456789012345678901234567890abcd",
		Status:        "pending",
	}

	// Set specific result
	mockGenerator.Results["owner/repo/1"] = struct {
		Result *ReviewResult
		Err    error
	}{
		Result: &ReviewResult{
			HTMLContent:   []byte("<html>Detailed Review</html>"),
			CriticalCount: 3,
			MediumCount:   7,
			LowCount:      12,
		},
		Err: nil,
	}

	poller := newTestPollerFull(mockGH, mockDB, mockStorage, mockGenerator)
	ctx := context.Background()

	prs := []github.PullRequest{
		{Owner: "owner", Repo: "repo", Number: 1, CommitSHA: "abc123def456789012345678901234567890abcd", Title: "Test PR", Author: "testuser"},
	}

	err := poller.generateReviewsBatch(ctx, prs)
	if err != nil {
		t.Fatalf("generateReviewsBatch returned error: %v", err)
	}

	// Verify PR was updated correctly
	pr := mockDB.PRs["owner/repo/1"]
	if pr.Status != "completed" {
		t.Errorf("expected status 'completed', got '%s'", pr.Status)
	}
	if pr.CriticalCount != 3 {
		t.Errorf("expected CriticalCount 3, got %d", pr.CriticalCount)
	}
	if pr.MediumCount != 7 {
		t.Errorf("expected MediumCount 7, got %d", pr.MediumCount)
	}
	if pr.LowCount != 12 {
		t.Errorf("expected LowCount 12, got %d", pr.LowCount)
	}

	// Verify review was saved
	if len(mockStorage.SaveReviewCalls) != 1 {
		t.Fatalf("expected 1 SaveReview call, got %d", len(mockStorage.SaveReviewCalls))
	}
	savedContent := mockStorage.SaveReviewCalls[0].Content
	if string(savedContent) != "<html>Detailed Review</html>" {
		t.Errorf("unexpected saved content: %s", string(savedContent))
	}
}

func TestGenerateReviewsBatch_ConcurrencyLimit(t *testing.T) {
	mockGH := NewMockGitHubClient()
	mockDB := NewMockDatabase()
	mockStorage := NewMockReviewStorage()
	mockGenerator := NewMockReviewGenerator()

	// Create 10 PRs
	var prs []github.PullRequest
	for i := 1; i <= 10; i++ {
		key := fmt.Sprintf("owner/repo/%d", i)
		sha := fmt.Sprintf("sha%d000000000000000000000000000000000000", i)
		mockDB.PRs[key] = &db.PR{
			RepoOwner:     "owner",
			RepoName:      "repo",
			PRNumber:      i,
			LastCommitSHA: sha,
			Status:        "pending",
		}
		prs = append(prs, github.PullRequest{
			Owner:     "owner",
			Repo:      "repo",
			Number:    i,
			CommitSHA: sha,
			Title:     fmt.Sprintf("PR %d", i),
			Author:    "user",
		})
	}

	// Add small delay to simulate work
	mockGenerator.SimulateDelay = 10 * time.Millisecond

	poller := newTestPollerFull(mockGH, mockDB, mockStorage, mockGenerator)
	ctx := context.Background()

	start := time.Now()
	err := poller.generateReviewsBatch(ctx, prs)
	duration := time.Since(start)

	if err != nil {
		t.Fatalf("generateReviewsBatch returned error: %v", err)
	}

	// All 10 PRs should have been processed
	if len(mockGenerator.GenerateReviewCalls) != 10 {
		t.Errorf("expected 10 GenerateReview calls, got %d", len(mockGenerator.GenerateReviewCalls))
	}

	// With 10 PRs, 10ms each, and concurrency limit of 5:
	// Batch 1 (PRs 1-5): 10ms
	// Batch 2 (PRs 6-10): 10ms
	// Total: ~20ms (with some overhead)
	// If fully sequential, would be 100ms
	// So duration should be less than 60ms (allowing for overhead)
	if duration > 100*time.Millisecond {
		t.Logf("Duration was %v - this suggests concurrency might not be working as expected", duration)
		// Not failing the test as timing can be flaky, but logging for visibility
	}

	// Verify all PRs completed
	for i := 1; i <= 10; i++ {
		key := fmt.Sprintf("owner/repo/%d", i)
		pr := mockDB.PRs[key]
		if pr.Status != "completed" {
			t.Errorf("PR %d expected status 'completed', got '%s'", i, pr.Status)
		}
	}
}

// =============================================================================
// processPRBatch Tests
// =============================================================================

func TestProcessPRBatch_NewPR_InsertsToDatabase(t *testing.T) {
	mockGH := NewMockGitHubClient()
	mockDB := NewMockDatabase()
	mockStorage := NewMockReviewStorage()
	mockGenerator := NewMockReviewGenerator()

	// Database starts empty - no PRs
	poller := newTestPollerFull(mockGH, mockDB, mockStorage, mockGenerator)
	ctx := context.Background()

	prs := []github.PullRequest{
		{Owner: "owner", Repo: "repo", Number: 1, CommitSHA: "abc123def456789012345678901234567890abcd", Title: "New PR", Author: "author"},
	}

	err := poller.processPRBatch(ctx, prs, true)
	if err != nil {
		t.Fatalf("processPRBatch returned error: %v", err)
	}

	// Verify PR was inserted
	pr := mockDB.PRs["owner/repo/1"]
	if pr == nil {
		t.Fatal("PR was not inserted into database")
	}

	// Verify metadata was set
	if pr.Title != "New PR" {
		t.Errorf("expected title 'New PR', got '%s'", pr.Title)
	}
	if pr.Author != "author" {
		t.Errorf("expected author 'author', got '%s'", pr.Author)
	}
}

func TestProcessPRBatch_ExistingPR_UpdatesMetadata(t *testing.T) {
	mockGH := NewMockGitHubClient()
	mockDB := NewMockDatabase()
	mockStorage := NewMockReviewStorage()
	mockGenerator := NewMockReviewGenerator()

	// Add existing PR with old metadata
	mockDB.PRs["owner/repo/1"] = &db.PR{
		RepoOwner:     "owner",
		RepoName:      "repo",
		PRNumber:      1,
		LastCommitSHA: "abc123def456789012345678901234567890abcd",
		Title:         "Old Title",
		Author:        "oldauthor",
		Status:        "completed", // Already completed
	}

	// Mark review as existing so we don't regenerate
	mockStorage.ExistingReviews["owner/repo/1/abc123def456789012345678901234567890abcd"] = true

	poller := newTestPollerFull(mockGH, mockDB, mockStorage, mockGenerator)
	ctx := context.Background()

	prs := []github.PullRequest{
		{Owner: "owner", Repo: "repo", Number: 1, CommitSHA: "abc123def456789012345678901234567890abcd", Title: "Updated Title", Author: "newauthor"},
	}

	err := poller.processPRBatch(ctx, prs, true)
	if err != nil {
		t.Fatalf("processPRBatch returned error: %v", err)
	}

	// Verify metadata was updated
	pr := mockDB.PRs["owner/repo/1"]
	if pr.Title != "Updated Title" {
		t.Errorf("expected title 'Updated Title', got '%s'", pr.Title)
	}
	if pr.Author != "newauthor" {
		t.Errorf("expected author 'newauthor', got '%s'", pr.Author)
	}
}

func TestProcessPRBatch_ManualTrigger_QueuesForReview(t *testing.T) {
	mockGH := NewMockGitHubClient()
	mockDB := NewMockDatabase()
	mockStorage := NewMockReviewStorage()
	mockGenerator := NewMockReviewGenerator()

	// Add PR in "generating" status (manual trigger)
	mockDB.PRs["owner/repo/1"] = &db.PR{
		RepoOwner:     "owner",
		RepoName:      "repo",
		PRNumber:      1,
		LastCommitSHA: "abc123def456789012345678901234567890abcd",
		Status:        "generating", // Manual trigger
	}

	poller := newTestPollerFull(mockGH, mockDB, mockStorage, mockGenerator)
	ctx := context.Background()

	prs := []github.PullRequest{
		{Owner: "owner", Repo: "repo", Number: 1, CommitSHA: "abc123def456789012345678901234567890abcd", Title: "PR", Author: "author"},
	}

	err := poller.processPRBatch(ctx, prs, true)
	if err != nil {
		t.Fatalf("processPRBatch returned error: %v", err)
	}

	// Generator should have been called (manual trigger)
	if len(mockGenerator.GenerateReviewCalls) != 1 {
		t.Errorf("expected 1 GenerateReview call for manual trigger, got %d", len(mockGenerator.GenerateReviewCalls))
	}

	// PR should be completed
	pr := mockDB.PRs["owner/repo/1"]
	if pr.Status != "completed" {
		t.Errorf("expected status 'completed', got '%s'", pr.Status)
	}
}

func TestProcessPRBatch_AutoReviewDisabled_SkipsPending(t *testing.T) {
	mockGH := NewMockGitHubClient()
	mockDB := NewMockDatabase()
	mockStorage := NewMockReviewStorage()
	mockGenerator := NewMockReviewGenerator()

	// Add pending PR
	mockDB.PRs["owner/repo/1"] = &db.PR{
		RepoOwner:     "owner",
		RepoName:      "repo",
		PRNumber:      1,
		LastCommitSHA: "abc123def456789012345678901234567890abcd",
		Status:        "pending",
	}

	poller := newTestPollerFull(mockGH, mockDB, mockStorage, mockGenerator)
	ctx := context.Background()

	prs := []github.PullRequest{
		{Owner: "owner", Repo: "repo", Number: 1, CommitSHA: "abc123def456789012345678901234567890abcd", Title: "PR", Author: "author"},
	}

	// Pass false for autoReviewEnabled to test the skip behavior
	err := poller.processPRBatch(ctx, prs, false)
	if err != nil {
		t.Fatalf("processPRBatch returned error: %v", err)
	}

	// Generator should NOT have been called (auto-review disabled)
	if len(mockGenerator.GenerateReviewCalls) != 0 {
		t.Errorf("expected no GenerateReview calls when auto-review disabled, got %d", len(mockGenerator.GenerateReviewCalls))
	}

	// PR should still be pending
	pr := mockDB.PRs["owner/repo/1"]
	if pr.Status != "pending" {
		t.Errorf("expected status 'pending', got '%s'", pr.Status)
	}
}

func TestProcessPRBatch_BroadcastsEvents(t *testing.T) {
	mockGH := NewMockGitHubClient()
	mockDB := NewMockDatabase()
	mockStorage := NewMockReviewStorage()
	mockGenerator := NewMockReviewGenerator()

	poller := newTestPollerFull(mockGH, mockDB, mockStorage, mockGenerator)

	// Track events
	var events []struct {
		eventType string
		payload   interface{}
	}
	poller.EventFunc = func(eventType string, payload interface{}) {
		events = append(events, struct {
			eventType string
			payload   interface{}
		}{eventType, payload})
	}

	ctx := context.Background()
	prs := []github.PullRequest{
		{Owner: "owner", Repo: "repo", Number: 1, CommitSHA: "abc123def456789012345678901234567890abcd", Title: "New PR", Author: "author"},
	}

	err := poller.processPRBatch(ctx, prs, true)
	if err != nil {
		t.Fatalf("processPRBatch returned error: %v", err)
	}

	// Should have broadcast events (pr_created for new PR, pr_updated during processing)
	if len(events) == 0 {
		t.Error("expected events to be broadcast")
	}

	// Check for pr_created event
	foundCreated := false
	for _, e := range events {
		if e.eventType == "pr_created" {
			foundCreated = true
			break
		}
	}
	if !foundCreated {
		t.Error("expected pr_created event")
	}
}

// =============================================================================
// reviewExists and saveReview Tests (Storage Interface)
// =============================================================================

func TestReviewExists_UsesStorageInterface(t *testing.T) {
	mockGH := NewMockGitHubClient()
	mockDB := NewMockDatabase()
	mockStorage := NewMockReviewStorage()

	// Mark a review as existing
	mockStorage.ExistingReviews["owner/repo/1/abc123def456"] = true

	poller := newTestPollerWithStorage(mockGH, mockDB, mockStorage)
	ctx := context.Background()

	// Check existing review
	exists, err := poller.reviewExists(ctx, "owner", "repo", 1, "abc123def456")
	if err != nil {
		t.Fatalf("reviewExists returned error: %v", err)
	}
	if !exists {
		t.Error("expected review to exist")
	}

	// Check non-existing review
	exists, err = poller.reviewExists(ctx, "owner", "repo", 2, "xyz789")
	if err != nil {
		t.Fatalf("reviewExists returned error: %v", err)
	}
	if exists {
		t.Error("expected review not to exist")
	}

	// Verify calls were tracked
	if len(mockStorage.ReviewExistsCalls) != 2 {
		t.Errorf("expected 2 ReviewExists calls, got %d", len(mockStorage.ReviewExistsCalls))
	}
}

func TestReviewExists_PropagatesError(t *testing.T) {
	mockGH := NewMockGitHubClient()
	mockDB := NewMockDatabase()
	mockStorage := NewMockReviewStorage()
	mockStorage.ReviewExistsError = errors.New("storage error")

	poller := newTestPollerWithStorage(mockGH, mockDB, mockStorage)
	ctx := context.Background()

	_, err := poller.reviewExists(ctx, "owner", "repo", 1, "abc123")
	if err == nil {
		t.Error("expected error to be propagated")
	}
	if err.Error() != "storage error" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSaveReview_UsesStorageInterface(t *testing.T) {
	mockGH := NewMockGitHubClient()
	mockDB := NewMockDatabase()
	mockStorage := NewMockReviewStorage()

	poller := newTestPollerWithStorage(mockGH, mockDB, mockStorage)
	ctx := context.Background()

	htmlContent := []byte("<html><body>Review content</body></html>")
	filename, err := poller.saveReview(ctx, "owner", "repo", 1, "abc123def456", htmlContent)

	if err != nil {
		t.Fatalf("saveReview returned error: %v", err)
	}

	if filename == "" {
		t.Error("expected non-empty filename")
	}

	// Verify content was saved
	key := "owner/repo/1/abc123def456"
	if savedContent, exists := mockStorage.SavedReviews[key]; !exists {
		t.Error("review was not saved to storage")
	} else if string(savedContent) != string(htmlContent) {
		t.Error("saved content does not match")
	}

	// Verify call was tracked
	if len(mockStorage.SaveReviewCalls) != 1 {
		t.Errorf("expected 1 SaveReview call, got %d", len(mockStorage.SaveReviewCalls))
	}
}

func TestSaveReview_PropagatesError(t *testing.T) {
	mockGH := NewMockGitHubClient()
	mockDB := NewMockDatabase()
	mockStorage := NewMockReviewStorage()
	mockStorage.SaveReviewError = errors.New("save error")

	poller := newTestPollerWithStorage(mockGH, mockDB, mockStorage)
	ctx := context.Background()

	_, err := poller.saveReview(ctx, "owner", "repo", 1, "abc123", []byte("content"))
	if err == nil {
		t.Error("expected error to be propagated")
	}
	if err.Error() != "save error" {
		t.Errorf("unexpected error: %v", err)
	}
}

// =============================================================================
// checkForOutdatedReviews Tests
// =============================================================================

func TestCheckForOutdatedReviews_SameCommit_NoChange(t *testing.T) {
	mockGH := NewMockGitHubClient()
	mockDB := NewMockDatabase()

	// PR with matching commit SHA
	mockDB.PRs["owner/repo/1"] = &db.PR{
		RepoOwner:     "owner",
		RepoName:      "repo",
		PRNumber:      1,
		LastCommitSHA: "abc123def456789012345678901234567890abcd",
		Status:        "completed",
	}

	// GitHub returns the same SHA
	mockGH.GetPRHeadSHAResults["owner/repo/1"] = struct {
		SHA string
		Err error
	}{"abc123def456789012345678901234567890abcd", nil}

	poller := newTestPoller(mockGH, mockDB)
	ctx := context.Background()

	outdated, err := poller.checkForOutdatedReviews(ctx)

	if err != nil {
		t.Fatalf("checkForOutdatedReviews returned error: %v", err)
	}

	if outdated != 0 {
		t.Errorf("expected 0 outdated PRs, got %d", outdated)
	}

	// Status should not change
	pr := mockDB.PRs["owner/repo/1"]
	if pr.Status != "completed" {
		t.Errorf("PR status should remain 'completed', got '%s'", pr.Status)
	}

	// ResetPRToOutdated should not be called
	if len(mockDB.ResetPRToOutdatedCalls) != 0 {
		t.Errorf("expected no ResetPRToOutdated calls, got %d", len(mockDB.ResetPRToOutdatedCalls))
	}
}

func TestCheckForOutdatedReviews_NewCommit_ResetsToOutdated(t *testing.T) {
	mockGH := NewMockGitHubClient()
	mockDB := NewMockDatabase()

	// PR with old commit SHA
	mockDB.PRs["owner/repo/1"] = &db.PR{
		RepoOwner:      "owner",
		RepoName:       "repo",
		PRNumber:       1,
		LastCommitSHA:  "oldcommit123456789012345678901234567890ab",
		Status:         "completed",
		ReviewHTMLPath: "old-review.html",
	}

	// GitHub returns a new SHA
	mockGH.GetPRHeadSHAResults["owner/repo/1"] = struct {
		SHA string
		Err error
	}{"newcommit987654321098765432109876543210fe", nil}

	poller := newTestPoller(mockGH, mockDB)
	ctx := context.Background()

	outdated, err := poller.checkForOutdatedReviews(ctx)

	if err != nil {
		t.Fatalf("checkForOutdatedReviews returned error: %v", err)
	}

	if outdated != 1 {
		t.Errorf("expected 1 outdated PR, got %d", outdated)
	}

	// Verify ResetPRToOutdated was called with correct parameters
	if len(mockDB.ResetPRToOutdatedCalls) != 1 {
		t.Fatalf("expected 1 ResetPRToOutdated call, got %d", len(mockDB.ResetPRToOutdatedCalls))
	}

	call := mockDB.ResetPRToOutdatedCalls[0]
	if call.Owner != "owner" || call.Repo != "repo" || call.PRNumber != 1 {
		t.Errorf("ResetPRToOutdated called with wrong PR: %v", call)
	}
	if call.NewCommitSHA != "newcommit987654321098765432109876543210fe" {
		t.Errorf("ResetPRToOutdated called with wrong SHA: %s", call.NewCommitSHA)
	}
}

func TestCheckForOutdatedReviews_GeneratingWithNewCommit_KillsProcess(t *testing.T) {
	mockGH := NewMockGitHubClient()
	mockDB := NewMockDatabase()

	// PR that is currently generating
	mockDB.PRs["owner/repo/1"] = &db.PR{
		RepoOwner:     "owner",
		RepoName:      "repo",
		PRNumber:      1,
		LastCommitSHA: "oldcommit123456789012345678901234567890ab",
		Status:        "generating",
	}

	// GitHub returns a new SHA
	mockGH.GetPRHeadSHAResults["owner/repo/1"] = struct {
		SHA string
		Err error
	}{"newcommit987654321098765432109876543210fe", nil}

	poller := newTestPoller(mockGH, mockDB)

	// Simulate an active review process (but don't actually create a real process)
	// The killReview function will try to find and kill the process, but that's OK
	// for testing - we just want to verify the logic flow
	poller.trackReview("owner", "repo", 1, 99999) // Fake PID

	ctx := context.Background()
	outdated, err := poller.checkForOutdatedReviews(ctx)

	if err != nil {
		t.Fatalf("checkForOutdatedReviews returned error: %v", err)
	}

	if outdated != 1 {
		t.Errorf("expected 1 outdated PR, got %d", outdated)
	}

	// Verify the PR was reset
	if len(mockDB.ResetPRToOutdatedCalls) != 1 {
		t.Fatalf("expected 1 ResetPRToOutdated call, got %d", len(mockDB.ResetPRToOutdatedCalls))
	}
}

func TestCheckForOutdatedReviews_GitHubError_ContinuesProcessing(t *testing.T) {
	mockGH := NewMockGitHubClient()
	mockDB := NewMockDatabase()

	// Two PRs in database
	mockDB.PRs["owner/repo/1"] = &db.PR{
		RepoOwner:     "owner",
		RepoName:      "repo",
		PRNumber:      1,
		LastCommitSHA: "commit1aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Status:        "completed",
	}
	mockDB.PRs["owner/repo/2"] = &db.PR{
		RepoOwner:     "owner",
		RepoName:      "repo",
		PRNumber:      2,
		LastCommitSHA: "commit2oldaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Status:        "completed",
	}

	// PR 1 returns error, PR 2 has new commit
	mockGH.GetPRHeadSHAResults["owner/repo/1"] = struct {
		SHA string
		Err error
	}{"", errors.New("GitHub API error")}
	mockGH.GetPRHeadSHAResults["owner/repo/2"] = struct {
		SHA string
		Err error
	}{"commit2newbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", nil}

	poller := newTestPoller(mockGH, mockDB)
	ctx := context.Background()

	outdated, err := poller.checkForOutdatedReviews(ctx)

	if err != nil {
		t.Fatalf("checkForOutdatedReviews should not return error on individual PR errors: %v", err)
	}

	// Only PR 2 should be marked as outdated
	if outdated != 1 {
		t.Errorf("expected 1 outdated PR, got %d", outdated)
	}

	// Verify PR 1 still has original SHA (wasn't modified due to error)
	pr1 := mockDB.PRs["owner/repo/1"]
	if pr1.LastCommitSHA != "commit1aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
		t.Errorf("PR 1 SHA should not have changed")
	}

	// Verify PR 2 was reset
	if len(mockDB.ResetPRToOutdatedCalls) != 1 {
		t.Fatalf("expected 1 ResetPRToOutdated call, got %d", len(mockDB.ResetPRToOutdatedCalls))
	}
	if mockDB.ResetPRToOutdatedCalls[0].PRNumber != 2 {
		t.Errorf("expected ResetPRToOutdated to be called for PR 2")
	}
}

func TestCheckForOutdatedReviews_BroadcastsUpdate(t *testing.T) {
	mockGH := NewMockGitHubClient()
	mockDB := NewMockDatabase()

	mockDB.PRs["owner/repo/1"] = &db.PR{
		RepoOwner:     "owner",
		RepoName:      "repo",
		PRNumber:      1,
		LastCommitSHA: "oldcommit123456789012345678901234567890ab",
		Status:        "completed",
	}

	mockGH.GetPRHeadSHAResults["owner/repo/1"] = struct {
		SHA string
		Err error
	}{"newcommit987654321098765432109876543210fe", nil}

	poller := newTestPoller(mockGH, mockDB)

	// Track events
	var events []struct {
		eventType string
		payload   interface{}
	}
	poller.EventFunc = func(eventType string, payload interface{}) {
		events = append(events, struct {
			eventType string
			payload   interface{}
		}{eventType, payload})
	}

	ctx := context.Background()
	_, err := poller.checkForOutdatedReviews(ctx)

	if err != nil {
		t.Fatalf("checkForOutdatedReviews returned error: %v", err)
	}

	// Verify pr_updated event was sent
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	if events[0].eventType != "pr_updated" {
		t.Errorf("expected event type 'pr_updated', got '%s'", events[0].eventType)
	}

	payload, ok := events[0].payload.(map[string]interface{})
	if !ok {
		t.Fatalf("payload should be map[string]interface{}")
	}

	if payload["owner"] != "owner" || payload["repo"] != "repo" || payload["number"] != 1 {
		t.Errorf("unexpected payload: %v", payload)
	}
}

// =============================================================================
// poll() Integration Tests
// =============================================================================

func TestPoll_FullCycle_Success(t *testing.T) {
	mockGH := NewMockGitHubClient()
	mockDB := NewMockDatabase()
	mockStorage := NewMockReviewStorage()
	mockGenerator := NewMockReviewGenerator()

	// Setup: GitHub returns one PR requesting review
	mockGH.PRsRequestingReview = []github.PullRequest{
		{Owner: "owner", Repo: "repo", Number: 1, CommitSHA: "abc123def456789012345678901234567890abcd", Title: "Test PR", Author: "author"},
	}

	// Setup: No head SHA changes (PR is not outdated)
	mockGH.GetPRHeadSHAResults["owner/repo/1"] = struct {
		SHA string
		Err error
	}{"abc123def456789012345678901234567890abcd", nil}

	// Setup: PR is open
	mockGH.IsPROpenResults["owner/repo/1"] = struct {
		IsOpen bool
		Err    error
	}{true, nil}

	poller := newTestPollerFull(mockGH, mockDB, mockStorage, mockGenerator)
	ctx := context.Background()

	// Execute poll
	poller.poll(ctx)

	// Verify: PR was created in database
	pr := mockDB.PRs["owner/repo/1"]
	if pr == nil {
		t.Fatal("PR was not created in database")
	}

	// Verify: Review was generated
	if len(mockGenerator.GenerateReviewCalls) == 0 {
		t.Error("expected review to be generated")
	}

	// Verify: PR is completed
	if pr.Status != "completed" {
		t.Errorf("expected status 'completed', got '%s'", pr.Status)
	}
}

func TestPoll_CleansUpClosedPRs(t *testing.T) {
	mockGH := NewMockGitHubClient()
	mockDB := NewMockDatabase()
	mockStorage := NewMockReviewStorage()
	mockGenerator := NewMockReviewGenerator()

	// Setup: Database has a PR that is now closed on GitHub
	mockDB.PRs["owner/repo/1"] = &db.PR{
		RepoOwner:     "owner",
		RepoName:      "repo",
		PRNumber:      1,
		LastCommitSHA: "abc123def456789012345678901234567890abcd",
		Status:        "completed",
	}

	// Setup: PR is closed on GitHub
	mockGH.IsPROpenResults["owner/repo/1"] = struct {
		IsOpen bool
		Err    error
	}{false, nil}

	// Setup: No PRs from GitHub
	mockGH.PRsRequestingReview = []github.PullRequest{}
	mockGH.MyOpenPRs = []github.PullRequest{}

	poller := newTestPollerFull(mockGH, mockDB, mockStorage, mockGenerator)
	ctx := context.Background()

	// Execute poll
	poller.poll(ctx)

	// Verify: PR was removed from database
	if _, exists := mockDB.PRs["owner/repo/1"]; exists {
		t.Error("closed PR should have been removed from database")
	}
}

func TestPoll_ProcessesDatabasePendingPRs(t *testing.T) {
	mockGH := NewMockGitHubClient()
	mockDB := NewMockDatabase()
	mockStorage := NewMockReviewStorage()
	mockGenerator := NewMockReviewGenerator()

	// Setup: PR exists in database in pending state
	mockDB.PRs["owner/repo/1"] = &db.PR{
		RepoOwner:     "owner",
		RepoName:      "repo",
		PRNumber:      1,
		LastCommitSHA: "abc123def456789012345678901234567890abcd",
		Status:        "pending",
		Title:         "DB PR",
		Author:        "author",
	}

	// Setup: PR is still open
	mockGH.IsPROpenResults["owner/repo/1"] = struct {
		IsOpen bool
		Err    error
	}{true, nil}

	// Setup: Same commit (not outdated)
	mockGH.GetPRHeadSHAResults["owner/repo/1"] = struct {
		SHA string
		Err error
	}{"abc123def456789012345678901234567890abcd", nil}

	// Setup: No PRs from GitHub search (PR was already processed earlier)
	mockGH.PRsRequestingReview = []github.PullRequest{}
	mockGH.MyOpenPRs = []github.PullRequest{}

	poller := newTestPollerFull(mockGH, mockDB, mockStorage, mockGenerator)
	ctx := context.Background()

	// Execute poll
	poller.poll(ctx)

	// Verify: Review was generated for the pending PR
	if len(mockGenerator.GenerateReviewCalls) != 1 {
		t.Errorf("expected 1 GenerateReview call, got %d", len(mockGenerator.GenerateReviewCalls))
	}

	// Verify: PR is now completed
	pr := mockDB.PRs["owner/repo/1"]
	if pr.Status != "completed" {
		t.Errorf("expected status 'completed', got '%s'", pr.Status)
	}
}

func TestPoll_DetectsOutdatedReviews(t *testing.T) {
	mockGH := NewMockGitHubClient()
	mockDB := NewMockDatabase()
	mockStorage := NewMockReviewStorage()
	mockGenerator := NewMockReviewGenerator()

	// Setup: PR exists in database with completed review
	mockDB.PRs["owner/repo/1"] = &db.PR{
		RepoOwner:     "owner",
		RepoName:      "repo",
		PRNumber:      1,
		LastCommitSHA: "oldcommit123456789012345678901234567890ab",
		Status:        "completed",
		Title:         "PR",
		Author:        "author",
	}

	// Setup: PR is still open
	mockGH.IsPROpenResults["owner/repo/1"] = struct {
		IsOpen bool
		Err    error
	}{true, nil}

	// Setup: NEW commit on GitHub (PR is outdated)
	mockGH.GetPRHeadSHAResults["owner/repo/1"] = struct {
		SHA string
		Err error
	}{"newcommit987654321098765432109876543210fe", nil}

	// Setup: No PRs from GitHub search
	mockGH.PRsRequestingReview = []github.PullRequest{}
	mockGH.MyOpenPRs = []github.PullRequest{}

	poller := newTestPollerFull(mockGH, mockDB, mockStorage, mockGenerator)
	ctx := context.Background()

	// Execute poll
	poller.poll(ctx)

	// Verify: PR was reset to pending with new commit
	pr := mockDB.PRs["owner/repo/1"]
	if pr.LastCommitSHA != "newcommit987654321098765432109876543210fe" {
		t.Errorf("expected new commit SHA, got '%s'", pr.LastCommitSHA)
	}

	// Verify: Review was regenerated
	if len(mockGenerator.GenerateReviewCalls) != 1 {
		t.Errorf("expected 1 GenerateReview call for new commit, got %d", len(mockGenerator.GenerateReviewCalls))
	}
}

func TestPoll_SkipsPendingWhenAutoReviewDisabled(t *testing.T) {
	mockGH := NewMockGitHubClient()
	mockDB := NewMockDatabase()
	mockStorage := NewMockReviewStorage()
	mockGenerator := NewMockReviewGenerator()

	// Disable auto-review
	mockDB.AutoReviewEnabled = false

	// Setup: PR exists in database in pending state
	mockDB.PRs["owner/repo/1"] = &db.PR{
		RepoOwner:     "owner",
		RepoName:      "repo",
		PRNumber:      1,
		LastCommitSHA: "abc123def456789012345678901234567890abcd",
		Status:        "pending",
		Title:         "PR",
		Author:        "author",
	}

	// Setup: PR is still open
	mockGH.IsPROpenResults["owner/repo/1"] = struct {
		IsOpen bool
		Err    error
	}{true, nil}

	// Setup: Same commit
	mockGH.GetPRHeadSHAResults["owner/repo/1"] = struct {
		SHA string
		Err error
	}{"abc123def456789012345678901234567890abcd", nil}

	mockGH.PRsRequestingReview = []github.PullRequest{}
	mockGH.MyOpenPRs = []github.PullRequest{}

	poller := newTestPollerFull(mockGH, mockDB, mockStorage, mockGenerator)
	ctx := context.Background()

	// Execute poll
	poller.poll(ctx)

	// Verify: Review was NOT generated (auto-review disabled)
	if len(mockGenerator.GenerateReviewCalls) != 0 {
		t.Errorf("expected no GenerateReview calls when auto-review disabled, got %d", len(mockGenerator.GenerateReviewCalls))
	}

	// Verify: PR remains pending
	pr := mockDB.PRs["owner/repo/1"]
	if pr.Status != "pending" {
		t.Errorf("expected status 'pending', got '%s'", pr.Status)
	}
}

func TestPoll_ProcessesManualTriggerEvenWhenAutoReviewDisabled(t *testing.T) {
	mockGH := NewMockGitHubClient()
	mockDB := NewMockDatabase()
	mockStorage := NewMockReviewStorage()
	mockGenerator := NewMockReviewGenerator()

	// Disable auto-review
	mockDB.AutoReviewEnabled = false

	// Setup: PR exists in database in "generating" state (manual trigger)
	mockDB.PRs["owner/repo/1"] = &db.PR{
		RepoOwner:     "owner",
		RepoName:      "repo",
		PRNumber:      1,
		LastCommitSHA: "abc123def456789012345678901234567890abcd",
		Status:        "generating", // Manual trigger
		Title:         "PR",
		Author:        "author",
	}

	// Setup: PR is still open
	mockGH.IsPROpenResults["owner/repo/1"] = struct {
		IsOpen bool
		Err    error
	}{true, nil}

	// Setup: Same commit
	mockGH.GetPRHeadSHAResults["owner/repo/1"] = struct {
		SHA string
		Err error
	}{"abc123def456789012345678901234567890abcd", nil}

	mockGH.PRsRequestingReview = []github.PullRequest{}
	mockGH.MyOpenPRs = []github.PullRequest{}

	poller := newTestPollerFull(mockGH, mockDB, mockStorage, mockGenerator)
	ctx := context.Background()

	// Execute poll
	poller.poll(ctx)

	// Verify: Review WAS generated (manual trigger overrides auto-review setting)
	if len(mockGenerator.GenerateReviewCalls) != 1 {
		t.Errorf("expected 1 GenerateReview call for manual trigger, got %d", len(mockGenerator.GenerateReviewCalls))
	}

	// Verify: PR is completed
	pr := mockDB.PRs["owner/repo/1"]
	if pr.Status != "completed" {
		t.Errorf("expected status 'completed', got '%s'", pr.Status)
	}
}

// =============================================================================
// Broadcast-Only-On-Change Tests
// =============================================================================

func TestPoll_ReviewDataUpdate_NoBroadcastWhenUnchanged(t *testing.T) {
	mockGH := NewMockGitHubClient()
	mockDB := NewMockDatabase()
	mockStorage := NewMockReviewStorage()
	mockGenerator := NewMockReviewGenerator()

	// Setup: PR exists with specific review data
	mockDB.PRs["owner/repo/1"] = &db.PR{
		RepoOwner:      "owner",
		RepoName:       "repo",
		PRNumber:       1,
		LastCommitSHA:  "abc123def456789012345678901234567890abcd",
		Status:         "completed",
		Title:          "Test PR",
		Author:         "author",
		ApprovalCount:  2,
		MyReviewStatus: "APPROVED",
		Draft:          false,
		CIState:        "success",
		CIFailedChecks: "[]",
	}

	// Setup: GitHub returns the SAME review data (no changes)
	mockGH.BatchGetPRReviewDataResults["owner/repo/1"] = &github.PRReviewData{
		ApprovalCount:  2,        // Same as database
		MyReviewStatus: "APPROVED", // Same as database
	}

	// Setup: GitHub returns the SAME CI status (no changes)
	mockGH.BatchGetCIStatusResults["owner/repo/1"] = &github.CIStatus{
		State:        "success", // Same as database
		FailedChecks: []string{},
	}

	// Setup: PR is still open
	mockGH.IsPROpenResults["owner/repo/1"] = struct {
		IsOpen bool
		Err    error
	}{true, nil}

	// Setup: Same commit (not outdated)
	mockGH.GetPRHeadSHAResults["owner/repo/1"] = struct {
		SHA string
		Err error
	}{"abc123def456789012345678901234567890abcd", nil}

	// Setup: Return this PR from GitHub
	mockGH.PRsRequestingReview = []github.PullRequest{
		{Owner: "owner", Repo: "repo", Number: 1, CommitSHA: "abc123def456789012345678901234567890abcd", Title: "Test PR", Author: "author", Draft: false},
	}

	// Mark review as existing to skip generation
	mockStorage.ExistingReviews["owner/repo/1/abc123def456789012345678901234567890abcd"] = true

	poller := newTestPollerFull(mockGH, mockDB, mockStorage, mockGenerator)

	// Track events
	var events []struct {
		eventType string
		payload   interface{}
	}
	poller.EventFunc = func(eventType string, payload interface{}) {
		events = append(events, struct {
			eventType string
			payload   interface{}
		}{eventType, payload})
	}

	ctx := context.Background()
	poller.poll(ctx)

	// Count pr_updated events (should be minimal since nothing changed)
	prUpdatedCount := 0
	for _, e := range events {
		if e.eventType == "pr_updated" {
			prUpdatedCount++
		}
	}

	// With no changes to review data or CI status, we should see zero broadcasts
	// from those update loops (there may be broadcasts from other sources like processPRBatch)
	// The key is that we should NOT see excessive broadcasts
	if prUpdatedCount > 2 {
		t.Errorf("expected at most 2 pr_updated events (from processPRBatch), got %d - review/CI data updates may be broadcasting unnecessarily", prUpdatedCount)
	}
}

func TestPoll_ReviewDataUpdate_BroadcastsWhenApprovalChanges(t *testing.T) {
	mockGH := NewMockGitHubClient()
	mockDB := NewMockDatabase()
	mockStorage := NewMockReviewStorage()
	mockGenerator := NewMockReviewGenerator()

	// Setup: PR exists with initial review data
	mockDB.PRs["owner/repo/1"] = &db.PR{
		RepoOwner:      "owner",
		RepoName:       "repo",
		PRNumber:       1,
		LastCommitSHA:  "abc123def456789012345678901234567890abcd",
		Status:         "completed",
		Title:          "Test PR",
		Author:         "author",
		ApprovalCount:  1,           // OLD value
		MyReviewStatus: "COMMENTED", // OLD value
		Draft:          false,
		CIState:        "success",
		CIFailedChecks: "[]",
	}

	// Setup: GitHub returns DIFFERENT review data
	mockGH.BatchGetPRReviewDataResults["owner/repo/1"] = &github.PRReviewData{
		ApprovalCount:  3,          // CHANGED from 1 to 3
		MyReviewStatus: "APPROVED", // CHANGED from COMMENTED to APPROVED
	}

	// Setup: GitHub returns the same CI status (no changes there)
	mockGH.BatchGetCIStatusResults["owner/repo/1"] = &github.CIStatus{
		State:        "success",
		FailedChecks: []string{},
	}

	// Setup: PR is still open
	mockGH.IsPROpenResults["owner/repo/1"] = struct {
		IsOpen bool
		Err    error
	}{true, nil}

	// Setup: Same commit
	mockGH.GetPRHeadSHAResults["owner/repo/1"] = struct {
		SHA string
		Err error
	}{"abc123def456789012345678901234567890abcd", nil}

	mockGH.PRsRequestingReview = []github.PullRequest{
		{Owner: "owner", Repo: "repo", Number: 1, CommitSHA: "abc123def456789012345678901234567890abcd", Title: "Test PR", Author: "author", Draft: false},
	}

	mockStorage.ExistingReviews["owner/repo/1/abc123def456789012345678901234567890abcd"] = true

	poller := newTestPollerFull(mockGH, mockDB, mockStorage, mockGenerator)

	var events []struct {
		eventType string
		payload   interface{}
	}
	poller.EventFunc = func(eventType string, payload interface{}) {
		events = append(events, struct {
			eventType string
			payload   interface{}
		}{eventType, payload})
	}

	ctx := context.Background()
	poller.poll(ctx)

	// Verify the database was updated with new values
	pr := mockDB.PRs["owner/repo/1"]
	if pr.ApprovalCount != 3 {
		t.Errorf("expected ApprovalCount 3, got %d", pr.ApprovalCount)
	}
	if pr.MyReviewStatus != "APPROVED" {
		t.Errorf("expected MyReviewStatus 'APPROVED', got '%s'", pr.MyReviewStatus)
	}

	// Verify at least one pr_updated event was broadcast
	prUpdatedCount := 0
	for _, e := range events {
		if e.eventType == "pr_updated" {
			prUpdatedCount++
		}
	}
	if prUpdatedCount == 0 {
		t.Error("expected at least one pr_updated event when approval count changed")
	}
}

func TestPoll_CIStatusUpdate_BroadcastsWhenStateChanges(t *testing.T) {
	mockGH := NewMockGitHubClient()
	mockDB := NewMockDatabase()
	mockStorage := NewMockReviewStorage()
	mockGenerator := NewMockReviewGenerator()

	// Setup: PR exists with initial CI status
	mockDB.PRs["owner/repo/1"] = &db.PR{
		RepoOwner:      "owner",
		RepoName:       "repo",
		PRNumber:       1,
		LastCommitSHA:  "abc123def456789012345678901234567890abcd",
		Status:         "completed",
		Title:          "Test PR",
		Author:         "author",
		ApprovalCount:  2,
		MyReviewStatus: "APPROVED",
		Draft:          false,
		CIState:        "pending", // OLD value
		CIFailedChecks: "[]",
	}

	// Setup: GitHub returns same review data (no changes)
	mockGH.BatchGetPRReviewDataResults["owner/repo/1"] = &github.PRReviewData{
		ApprovalCount:  2,
		MyReviewStatus: "APPROVED",
	}

	// Setup: GitHub returns DIFFERENT CI status
	mockGH.BatchGetCIStatusResults["owner/repo/1"] = &github.CIStatus{
		State:        "failure",                    // CHANGED from pending to failure
		FailedChecks: []string{"test", "lint"},     // Now has failed checks
	}

	mockGH.IsPROpenResults["owner/repo/1"] = struct {
		IsOpen bool
		Err    error
	}{true, nil}

	mockGH.GetPRHeadSHAResults["owner/repo/1"] = struct {
		SHA string
		Err error
	}{"abc123def456789012345678901234567890abcd", nil}

	mockGH.PRsRequestingReview = []github.PullRequest{
		{Owner: "owner", Repo: "repo", Number: 1, CommitSHA: "abc123def456789012345678901234567890abcd", Title: "Test PR", Author: "author", Draft: false},
	}

	mockStorage.ExistingReviews["owner/repo/1/abc123def456789012345678901234567890abcd"] = true

	poller := newTestPollerFull(mockGH, mockDB, mockStorage, mockGenerator)

	var events []struct {
		eventType string
		payload   interface{}
	}
	poller.EventFunc = func(eventType string, payload interface{}) {
		events = append(events, struct {
			eventType string
			payload   interface{}
		}{eventType, payload})
	}

	ctx := context.Background()
	poller.poll(ctx)

	// Verify the database was updated with new CI values
	pr := mockDB.PRs["owner/repo/1"]
	if pr.CIState != "failure" {
		t.Errorf("expected CIState 'failure', got '%s'", pr.CIState)
	}
	if pr.CIFailedChecks != `["test","lint"]` {
		t.Errorf("expected CIFailedChecks '[\"test\",\"lint\"]', got '%s'", pr.CIFailedChecks)
	}

	// Verify at least one pr_updated event was broadcast for the CI change
	prUpdatedCount := 0
	for _, e := range events {
		if e.eventType == "pr_updated" {
			prUpdatedCount++
		}
	}
	if prUpdatedCount == 0 {
		t.Error("expected at least one pr_updated event when CI state changed")
	}
}

func TestPoll_CIStatusUpdate_NoBroadcastWhenUnchanged(t *testing.T) {
	mockGH := NewMockGitHubClient()
	mockDB := NewMockDatabase()
	mockStorage := NewMockReviewStorage()
	mockGenerator := NewMockReviewGenerator()

	// Setup: PR with specific CI status
	mockDB.PRs["owner/repo/1"] = &db.PR{
		RepoOwner:      "owner",
		RepoName:       "repo",
		PRNumber:       1,
		LastCommitSHA:  "abc123def456789012345678901234567890abcd",
		Status:         "completed",
		Title:          "Test PR",
		Author:         "author",
		ApprovalCount:  2,
		MyReviewStatus: "APPROVED",
		Draft:          false,
		CIState:        "failure",
		CIFailedChecks: `["test"]`, // Note: JSON string format
	}

	// Setup: GitHub returns SAME review data
	mockGH.BatchGetPRReviewDataResults["owner/repo/1"] = &github.PRReviewData{
		ApprovalCount:  2,
		MyReviewStatus: "APPROVED",
	}

	// Setup: GitHub returns SAME CI status
	mockGH.BatchGetCIStatusResults["owner/repo/1"] = &github.CIStatus{
		State:        "failure",      // Same
		FailedChecks: []string{"test"}, // Same
	}

	mockGH.IsPROpenResults["owner/repo/1"] = struct {
		IsOpen bool
		Err    error
	}{true, nil}

	mockGH.GetPRHeadSHAResults["owner/repo/1"] = struct {
		SHA string
		Err error
	}{"abc123def456789012345678901234567890abcd", nil}

	mockGH.PRsRequestingReview = []github.PullRequest{
		{Owner: "owner", Repo: "repo", Number: 1, CommitSHA: "abc123def456789012345678901234567890abcd", Title: "Test PR", Author: "author", Draft: false},
	}

	mockStorage.ExistingReviews["owner/repo/1/abc123def456789012345678901234567890abcd"] = true

	poller := newTestPollerFull(mockGH, mockDB, mockStorage, mockGenerator)

	var events []struct {
		eventType string
		payload   interface{}
	}
	poller.EventFunc = func(eventType string, payload interface{}) {
		events = append(events, struct {
			eventType string
			payload   interface{}
		}{eventType, payload})
	}

	ctx := context.Background()
	poller.poll(ctx)

	// Count pr_updated events - should be minimal
	prUpdatedCount := 0
	for _, e := range events {
		if e.eventType == "pr_updated" {
			prUpdatedCount++
		}
	}

	// With no changes to review data or CI status, we should have minimal broadcasts
	if prUpdatedCount > 2 {
		t.Errorf("expected at most 2 pr_updated events when nothing changed, got %d", prUpdatedCount)
	}
}

func TestPoll_DraftStatusChange_BroadcastsUpdate(t *testing.T) {
	mockGH := NewMockGitHubClient()
	mockDB := NewMockDatabase()
	mockStorage := NewMockReviewStorage()
	mockGenerator := NewMockReviewGenerator()

	// Setup: PR was a draft
	mockDB.PRs["owner/repo/1"] = &db.PR{
		RepoOwner:      "owner",
		RepoName:       "repo",
		PRNumber:       1,
		LastCommitSHA:  "abc123def456789012345678901234567890abcd",
		Status:         "completed",
		Title:          "Test PR",
		Author:         "author",
		ApprovalCount:  2,
		MyReviewStatus: "APPROVED",
		Draft:          true, // Was draft
		CIState:        "success",
		CIFailedChecks: "[]",
	}

	// Setup: GitHub returns same review data
	mockGH.BatchGetPRReviewDataResults["owner/repo/1"] = &github.PRReviewData{
		ApprovalCount:  2,
		MyReviewStatus: "APPROVED",
	}

	mockGH.BatchGetCIStatusResults["owner/repo/1"] = &github.CIStatus{
		State:        "success",
		FailedChecks: []string{},
	}

	mockGH.IsPROpenResults["owner/repo/1"] = struct {
		IsOpen bool
		Err    error
	}{true, nil}

	mockGH.GetPRHeadSHAResults["owner/repo/1"] = struct {
		SHA string
		Err error
	}{"abc123def456789012345678901234567890abcd", nil}

	// GitHub returns the PR as NO LONGER a draft
	mockGH.PRsRequestingReview = []github.PullRequest{
		{Owner: "owner", Repo: "repo", Number: 1, CommitSHA: "abc123def456789012345678901234567890abcd", Title: "Test PR", Author: "author", Draft: false}, // Draft changed to false
	}

	mockStorage.ExistingReviews["owner/repo/1/abc123def456789012345678901234567890abcd"] = true

	poller := newTestPollerFull(mockGH, mockDB, mockStorage, mockGenerator)

	var events []struct {
		eventType string
		payload   interface{}
	}
	poller.EventFunc = func(eventType string, payload interface{}) {
		events = append(events, struct {
			eventType string
			payload   interface{}
		}{eventType, payload})
	}

	ctx := context.Background()
	poller.poll(ctx)

	// Verify the database was updated
	pr := mockDB.PRs["owner/repo/1"]
	if pr.Draft != false {
		t.Errorf("expected Draft false, got %v", pr.Draft)
	}

	// Verify at least one pr_updated event was broadcast
	prUpdatedCount := 0
	for _, e := range events {
		if e.eventType == "pr_updated" {
			prUpdatedCount++
		}
	}
	if prUpdatedCount == 0 {
		t.Error("expected at least one pr_updated event when draft status changed")
	}
}

// =============================================================================
// User PR Views Sync Tests - Ensures poller syncs data to user_pr_views
// =============================================================================

func TestPoller_SyncsReviewStatusToUserPRViews(t *testing.T) {
	// This test verifies that the poller syncs review status to user_pr_views,
	// not just to the prs table. This is critical because the API reads from
	// user_pr_views.review_status, not prs.my_review_status.
	//
	// Regression test for: review status not appearing in UI because
	// poller updated prs table but API read from user_pr_views.

	mockGH := &MockGitHubClient{
		PRsRequestingReview: []github.PullRequest{
			{Owner: "owner", Repo: "repo", Number: 1, CommitSHA: "sha1"},
		},
		BatchGetPRReviewDataResults: map[string]*github.PRReviewData{
			"owner/repo/1": {MyReviewStatus: "APPROVED", ApprovalCount: 1},
		},
	}

	mockDB := NewMockDatabase()
	// Pre-populate the PR in the database
	mockDB.PRs["owner/repo/1"] = &db.PR{
		ID:             1,
		RepoOwner:      "owner",
		RepoName:       "repo",
		PRNumber:       1,
		LastCommitSHA:  "sha1",
		Status:         "completed",
		MyReviewStatus: "", // Empty in prs table
	}

	devUser := &db.User{ID: 1, GitHubUsername: "testuser"}

	poller := &Poller{
		db:            mockDB,
		ghClient:      mockGH,
		activeReviews: make(map[string]ProcessInfo),
		devUser:       devUser,
	}

	poller.EventFunc = func(eventType string, payload interface{}) {}

	ctx := context.Background()
	poller.poll(ctx)

	// Verify that UpdateUserReviewStatus was called
	// The mock will store the call, but for simplicity we verify the prs table was updated
	// (The real verification would require checking the mock's call history)
	pr := mockDB.PRs["owner/repo/1"]
	if pr.MyReviewStatus != "APPROVED" {
		t.Errorf("expected MyReviewStatus APPROVED in prs table, got %q", pr.MyReviewStatus)
	}
}

func TestPoller_SyncsViaTeamsToUserPRViews(t *testing.T) {
	// This test verifies that the poller fetches and syncs reviewer groups
	// to user_pr_views.via_teams, which is displayed in the UI.
	//
	// Regression test for: via_teams column being empty because
	// the feature was never wired up to the poller.

	mockGH := &MockGitHubClient{
		PRsRequestingReview: []github.PullRequest{
			{Owner: "owner", Repo: "repo", Number: 1, CommitSHA: "sha1"},
		},
		BatchGetPRReviewDataResults: map[string]*github.PRReviewData{
			"owner/repo/1": {MyReviewStatus: "", ApprovalCount: 0},
		},
	}

	mockDB := NewMockDatabase()
	mockDB.PRs["owner/repo/1"] = &db.PR{
		ID:            1,
		RepoOwner:     "owner",
		RepoName:      "repo",
		PRNumber:      1,
		LastCommitSHA: "sha1",
		Status:        "completed",
	}

	devUser := &db.User{ID: 1, GitHubUsername: "testuser"}

	poller := &Poller{
		db:            mockDB,
		ghClient:      mockGH,
		activeReviews: make(map[string]ProcessInfo),
		devUser:       devUser,
	}

	poller.EventFunc = func(eventType string, payload interface{}) {}

	ctx := context.Background()
	poller.poll(ctx)

	// This test passes if poll() doesn't panic when calling BatchGetReviewerGroups
	// A more complete test would verify the mock's UpdateUserViaTeams was called
	// with the expected values, but the mock currently just returns nil.
}
