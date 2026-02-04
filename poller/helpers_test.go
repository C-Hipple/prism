package poller

import (
	"testing"
	"time"

	"pr-review-server/db"
	"pr-review-server/github"
)

func TestMergePRLists_NoDuplicates(t *testing.T) {
	reviewPRs := []github.PullRequest{
		{Owner: "owner", Repo: "repo", Number: 1, CommitSHA: "sha1"},
		{Owner: "owner", Repo: "repo", Number: 2, CommitSHA: "sha2"},
	}
	myPRs := []github.PullRequest{
		{Owner: "owner", Repo: "repo", Number: 2, CommitSHA: "sha2"}, // Duplicate
		{Owner: "owner", Repo: "repo", Number: 3, CommitSHA: "sha3"},
	}
	dbPRs := []db.PR{
		{RepoOwner: "owner", RepoName: "repo", PRNumber: 1, LastCommitSHA: "sha1"}, // Duplicate
		{RepoOwner: "owner", RepoName: "repo", PRNumber: 4, LastCommitSHA: "sha4"},
	}

	result := mergePRLists(reviewPRs, myPRs, dbPRs)

	// Should have 4 unique PRs (1, 2, 3, 4)
	if len(result) != 4 {
		t.Errorf("expected 4 unique PRs, got %d", len(result))
	}

	// Verify all expected PRs are present
	expected := map[int]bool{1: false, 2: false, 3: false, 4: false}
	for _, pr := range result {
		expected[pr.Number] = true
	}
	for num, found := range expected {
		if !found {
			t.Errorf("PR #%d was not found in result", num)
		}
	}
}

func TestMergePRLists_EmptyInputs(t *testing.T) {
	result := mergePRLists(nil, nil, nil)
	if len(result) != 0 {
		t.Errorf("expected empty result for empty inputs, got %d PRs", len(result))
	}

	// Test with just one non-empty input
	reviewPRs := []github.PullRequest{
		{Owner: "owner", Repo: "repo", Number: 1, CommitSHA: "sha1"},
	}
	result = mergePRLists(reviewPRs, nil, nil)
	if len(result) != 1 {
		t.Errorf("expected 1 PR, got %d", len(result))
	}
}

func TestGroupPRsByRepo_GroupsCorrectly(t *testing.T) {
	prs := []github.PullRequest{
		{Owner: "owner1", Repo: "repo1", Number: 1},
		{Owner: "owner1", Repo: "repo1", Number: 2},
		{Owner: "owner1", Repo: "repo2", Number: 3},
		{Owner: "owner2", Repo: "repo1", Number: 4},
	}

	grouped := groupPRsByRepo(prs)

	// Should have 3 groups
	if len(grouped) != 3 {
		t.Errorf("expected 3 groups, got %d", len(grouped))
	}

	// Check owner1/repo1 group
	if len(grouped["owner1/repo1"]) != 2 {
		t.Errorf("expected 2 PRs in owner1/repo1, got %d", len(grouped["owner1/repo1"]))
	}

	// Check owner1/repo2 group
	if len(grouped["owner1/repo2"]) != 1 {
		t.Errorf("expected 1 PR in owner1/repo2, got %d", len(grouped["owner1/repo2"]))
	}

	// Check owner2/repo1 group
	if len(grouped["owner2/repo1"]) != 1 {
		t.Errorf("expected 1 PR in owner2/repo1, got %d", len(grouped["owner2/repo1"]))
	}
}

func TestGroupPRsByRepo_EmptyInput(t *testing.T) {
	grouped := groupPRsByRepo(nil)
	if len(grouped) != 0 {
		t.Errorf("expected empty map for nil input, got %d groups", len(grouped))
	}

	grouped = groupPRsByRepo([]github.PullRequest{})
	if len(grouped) != 0 {
		t.Errorf("expected empty map for empty input, got %d groups", len(grouped))
	}
}

func TestBuildPRFromDB_ConvertsAllFields(t *testing.T) {
	createdAt := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	dbPR := db.PR{
		RepoOwner:     "owner",
		RepoName:      "repo",
		PRNumber:      123,
		LastCommitSHA: "abc123def456",
		Title:         "Test PR Title",
		Author:        "testauthor",
		CreatedAt:     &createdAt,
		Draft:         true,
	}

	ghPR := buildPRFromDB(dbPR)

	if ghPR.Owner != "owner" {
		t.Errorf("expected Owner 'owner', got '%s'", ghPR.Owner)
	}
	if ghPR.Repo != "repo" {
		t.Errorf("expected Repo 'repo', got '%s'", ghPR.Repo)
	}
	if ghPR.Number != 123 {
		t.Errorf("expected Number 123, got %d", ghPR.Number)
	}
	if ghPR.CommitSHA != "abc123def456" {
		t.Errorf("expected CommitSHA 'abc123def456', got '%s'", ghPR.CommitSHA)
	}
	if ghPR.Title != "Test PR Title" {
		t.Errorf("expected Title 'Test PR Title', got '%s'", ghPR.Title)
	}
	if ghPR.Author != "testauthor" {
		t.Errorf("expected Author 'testauthor', got '%s'", ghPR.Author)
	}
	if ghPR.URL != "https://github.com/owner/repo/pull/123" {
		t.Errorf("expected URL 'https://github.com/owner/repo/pull/123', got '%s'", ghPR.URL)
	}
	if ghPR.CreatedAt == nil || !ghPR.CreatedAt.Equal(createdAt) {
		t.Errorf("expected CreatedAt %v, got %v", createdAt, ghPR.CreatedAt)
	}
	if !ghPR.Draft {
		t.Errorf("expected Draft true, got false")
	}
}

func TestShouldSkipDatabasePR_AutoReviewEnabled(t *testing.T) {
	// When auto-review is enabled, never skip pending PRs
	dbPR := db.PR{Status: "pending"}
	if shouldSkipDatabasePR(dbPR, true) {
		t.Error("should not skip pending PR when auto-review is enabled")
	}

	// Completed PRs are not skipped either (they just won't be re-processed)
	dbPR = db.PR{Status: "completed"}
	if shouldSkipDatabasePR(dbPR, true) {
		t.Error("should not skip completed PR when auto-review is enabled")
	}
}

func TestShouldSkipDatabasePR_AutoReviewDisabled(t *testing.T) {
	// When auto-review is disabled, skip pending PRs
	dbPR := db.PR{Status: "pending"}
	if !shouldSkipDatabasePR(dbPR, false) {
		t.Error("should skip pending PR when auto-review is disabled")
	}

	// But never skip generating PRs (manual triggers)
	dbPR = db.PR{Status: "generating"}
	if shouldSkipDatabasePR(dbPR, false) {
		t.Error("should NOT skip generating PR even when auto-review is disabled")
	}

	// Completed PRs are not explicitly skipped (they're handled by shouldReview logic)
	dbPR = db.PR{Status: "completed"}
	if shouldSkipDatabasePR(dbPR, false) {
		t.Error("should not skip completed PR")
	}
}
