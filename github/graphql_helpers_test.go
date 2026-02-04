package github

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBuildPRAliasMap(t *testing.T) {
	prs := []PullRequest{
		{Number: 101},
		{Number: 102},
		{Number: 103},
	}

	aliases := buildPRAliasMap(prs)

	if len(aliases) != 3 {
		t.Errorf("Expected 3 aliases, got %d", len(aliases))
	}

	// Check mapping
	if aliases["pr0"] != 101 {
		t.Errorf("Expected pr0 -> 101, got %d", aliases["pr0"])
	}
	if aliases["pr1"] != 102 {
		t.Errorf("Expected pr1 -> 102, got %d", aliases["pr1"])
	}
	if aliases["pr2"] != 103 {
		t.Errorf("Expected pr2 -> 103, got %d", aliases["pr2"])
	}
}

func TestBuildPRInfoAliasMap(t *testing.T) {
	prs := []PRInfoWithCommit{
		{Owner: "owner1", Repo: "repo1", Number: 101, CommitSHA: "abc123"},
		{Owner: "owner2", Repo: "repo2", Number: 102, CommitSHA: "def456"},
	}

	aliases := buildPRInfoAliasMap(prs)

	if len(aliases) != 2 {
		t.Errorf("Expected 2 aliases, got %d", len(aliases))
	}

	// Check first mapping
	prInfo0 := aliases["pr0"]
	if prInfo0.Owner != "owner1" || prInfo0.Repo != "repo1" || prInfo0.Number != 101 {
		t.Errorf("Expected pr0 -> (owner1, repo1, 101), got (%s, %s, %d)", prInfo0.Owner, prInfo0.Repo, prInfo0.Number)
	}
}

func TestIsValidReviewState(t *testing.T) {
	tests := []struct {
		state    string
		expected bool
	}{
		{"APPROVED", true},
		{"CHANGES_REQUESTED", true},
		{"COMMENTED", true},
		{"PENDING", false},
		{"DISMISSED", false},
	}

	for _, tt := range tests {
		result := isValidReviewState(tt.state)
		if result != tt.expected {
			t.Errorf("isValidReviewState(%q) = %v, expected %v", tt.state, result, tt.expected)
		}
	}
}

func TestPrKey(t *testing.T) {
	key := prKey("owner", "repo", 123)
	expected := "owner/repo/123"
	if key != expected {
		t.Errorf("prKey(owner, repo, 123) = %q, expected %q", key, expected)
	}
}

func TestParseRepoFromURL(t *testing.T) {
	tests := []struct {
		url           string
		expectedOwner string
		expectedRepo  string
		expectError   bool
	}{
		{"https://api.github.com/repos/owner/repo", "owner", "repo", false},
		{"https://api.github.com/repos/org/project", "org", "project", false},
		{"x", "", "", true}, // Only 1 part when split by "/"
		{"", "", "", true},
	}

	for _, tt := range tests {
		owner, repo, err := parseRepoFromURL(tt.url)
		if tt.expectError {
			if err == nil {
				t.Errorf("parseRepoFromURL(%q) expected error, got nil", tt.url)
			}
		} else {
			if err != nil {
				t.Errorf("parseRepoFromURL(%q) unexpected error: %v", tt.url, err)
			}
			if owner != tt.expectedOwner || repo != tt.expectedRepo {
				t.Errorf("parseRepoFromURL(%q) = (%q, %q), expected (%q, %q)", tt.url, owner, repo, tt.expectedOwner, tt.expectedRepo)
			}
		}
	}
}

func TestCountUserApprovals(t *testing.T) {
	client := NewClient("token", "current-user")

	reviews := ReviewsData{
		Nodes: []ReviewNode{
			{Author: &ReviewAuthor{Login: "user1"}, State: "APPROVED"},
			{Author: &ReviewAuthor{Login: "user2"}, State: "CHANGES_REQUESTED"},
			{Author: &ReviewAuthor{Login: "current-user"}, State: "APPROVED"},
			{Author: &ReviewAuthor{Login: "user1"}, State: "COMMENTED"}, // user1's latest
			{Author: nil, State: "APPROVED"},                            // Bot/deleted user - should be skipped
			{Author: &ReviewAuthor{Login: "user3"}, State: "PENDING"},   // Should be skipped
		},
	}

	approvalCount, myReviewStatus := client.countUserApprovals(reviews)

	// user1: COMMENTED (latest), user2: CHANGES_REQUESTED, current-user: APPROVED
	// So only current-user has APPROVED
	if approvalCount != 1 {
		t.Errorf("Expected 1 approval, got %d", approvalCount)
	}

	if myReviewStatus != "APPROVED" {
		t.Errorf("Expected myReviewStatus to be APPROVED, got %q", myReviewStatus)
	}
}

func TestExtractReviewerGroups(t *testing.T) {
	client := NewClient("token", "current-user")

	tests := []struct {
		name     string
		requests ReviewRequestsData
		expected []string
	}{
		{
			name: "Team request only",
			requests: ReviewRequestsData{
				Nodes: []ReviewRequester{
					{RequestedReviewer: struct {
						TypeName string `json:"__typename"`
						Login    string `json:"login"`
						Name     string `json:"name"`
					}{TypeName: "Team", Name: "backend-team"}},
				},
			},
			expected: []string{"backend-team"},
		},
		{
			name: "Personal request only",
			requests: ReviewRequestsData{
				Nodes: []ReviewRequester{
					{RequestedReviewer: struct {
						TypeName string `json:"__typename"`
						Login    string `json:"login"`
						Name     string `json:"name"`
					}{TypeName: "User", Login: "current-user"}},
				},
			},
			expected: []string{"__PERSONAL__"},
		},
		{
			name: "Team and personal request - team takes precedence",
			requests: ReviewRequestsData{
				Nodes: []ReviewRequester{
					{RequestedReviewer: struct {
						TypeName string `json:"__typename"`
						Login    string `json:"login"`
						Name     string `json:"name"`
					}{TypeName: "User", Login: "current-user"}},
					{RequestedReviewer: struct {
						TypeName string `json:"__typename"`
						Login    string `json:"login"`
						Name     string `json:"name"`
					}{TypeName: "Team", Name: "frontend-team"}},
				},
			},
			expected: []string{"frontend-team"},
		},
		{
			name:     "No requests",
			requests: ReviewRequestsData{Nodes: []ReviewRequester{}},
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := client.extractReviewerGroups(tt.requests)
			if len(result) != len(tt.expected) {
				t.Errorf("Expected %v, got %v", tt.expected, result)
				return
			}
			for i, v := range result {
				if v != tt.expected[i] {
					t.Errorf("Expected %v, got %v", tt.expected, result)
					return
				}
			}
		})
	}
}

func TestParseCIStatusFromRollup(t *testing.T) {
	tests := []struct {
		name           string
		rollup         *StatusCheckRollup
		expectedState  string
		expectedFailed []string
	}{
		{
			name: "All success",
			rollup: &StatusCheckRollup{
				State: "SUCCESS",
				Contexts: ContextsData{
					Nodes: []CheckNode{
						{TypeName: "CheckRun", Name: "test", Status: "COMPLETED", Conclusion: "SUCCESS"},
						{TypeName: "CheckRun", Name: "lint", Status: "COMPLETED", Conclusion: "SUCCESS"},
					},
				},
			},
			expectedState:  "success",
			expectedFailed: nil,
		},
		{
			name: "Pending check",
			rollup: &StatusCheckRollup{
				State: "PENDING",
				Contexts: ContextsData{
					Nodes: []CheckNode{
						{TypeName: "CheckRun", Name: "test", Status: "IN_PROGRESS"},
					},
				},
			},
			expectedState:  "pending",
			expectedFailed: nil,
		},
		{
			name: "Failed check",
			rollup: &StatusCheckRollup{
				State: "FAILURE",
				Contexts: ContextsData{
					Nodes: []CheckNode{
						{TypeName: "CheckRun", Name: "test", Status: "COMPLETED", Conclusion: "FAILURE"},
						{TypeName: "CheckRun", Name: "lint", Status: "COMPLETED", Conclusion: "SUCCESS"},
					},
				},
			},
			expectedState:  "failure",
			expectedFailed: []string{"test"},
		},
		{
			name: "StatusContext failure",
			rollup: &StatusCheckRollup{
				State: "FAILURE",
				Contexts: ContextsData{
					Nodes: []CheckNode{
						{TypeName: "StatusContext", Context: "ci/build", State: "FAILURE"},
					},
				},
			},
			expectedState:  "failure",
			expectedFailed: []string{"ci/build"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state, failed := parseCIStatusFromRollup(tt.rollup)
			if state != tt.expectedState {
				t.Errorf("Expected state %q, got %q", tt.expectedState, state)
			}
			if len(failed) != len(tt.expectedFailed) {
				t.Errorf("Expected %d failed checks, got %d", len(tt.expectedFailed), len(failed))
				return
			}
			for i, f := range failed {
				if f != tt.expectedFailed[i] {
					t.Errorf("Expected failed[%d] = %q, got %q", i, tt.expectedFailed[i], f)
				}
			}
		})
	}
}

func TestExecuteGraphQL(t *testing.T) {
	mockResponse := `{"data": {"test": "value"}}`

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("Expected POST request, got %s", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("Expected Bearer test-token, got %s", r.Header.Get("Authorization"))
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Expected application/json, got %s", r.Header.Get("Content-Type"))
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(mockResponse))
	}))
	defer ts.Close()

	client := NewClient("test-token", "test-user")
	client.httpClient = &http.Client{
		Transport: &redirectTransport{targetURL: ts.URL},
	}

	var result struct {
		Data struct {
			Test string `json:"test"`
		} `json:"data"`
	}

	err := client.executeGraphQL(context.Background(), "query { test }", &result)
	if err != nil {
		t.Fatalf("executeGraphQL failed: %v", err)
	}

	if result.Data.Test != "value" {
		t.Errorf("Expected test = 'value', got %q", result.Data.Test)
	}
}

func TestExecuteGraphQL_Error(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	client := NewClient("test-token", "test-user")
	client.httpClient = &http.Client{
		Transport: &redirectTransport{targetURL: ts.URL},
	}

	var result struct{}
	err := client.executeGraphQL(context.Background(), "query { test }", &result)
	if err == nil {
		t.Error("Expected error for 500 response, got nil")
	}
}
