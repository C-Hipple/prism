package github

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"testing"
)

func TestBatchGetReviewerGroups(t *testing.T) {
	// Mock response data matches the timeline-based structure expected by client.go
	mockResponse := `{
		"data": {
			"pr0": {
				"pullRequest": {
					"number": 101,
					"timelineItems": {
						"nodes": [
							{
								"requestedReviewer": {
									"__typename": "Team",
									"name": "backend-team",
									"slug": "backend-team"
								}
							},
							{
								"requestedReviewer": {
									"__typename": "User",
									"login": "other-user"
								}
							}
						]
					}
				}
			},
			"pr1": {
				"pullRequest": {
					"number": 102,
					"timelineItems": {
						"nodes": [
							{
								"requestedReviewer": {
									"__typename": "User",
									"login": "test-user"
								}
							}
						]
					}
				}
			},
			"pr2": {
				"pullRequest": {
					"number": 103,
					"timelineItems": {
						"nodes": []
					}
				}
			}
		}
	}`

	// Start mock server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("Expected POST request, got %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte(mockResponse)); err != nil {
			t.Errorf("failed to write response: %v", err)
		}
	}))
	defer ts.Close()

	// Since we can't easily change the URL in the client code (it's hardcoded),
	// we'll actually use a custom Transport in the client's http.Client to redirect the request to our mock server.

	client := NewClient("dummy-token", "test-user")
	client.httpClient = &http.Client{
		Transport: &redirectTransport{
			targetURL: ts.URL,
		},
	}

	prs := []PullRequest{
		{Owner: "owner", Repo: "repo", Number: 101},
		{Owner: "owner", Repo: "repo", Number: 102},
		{Owner: "owner", Repo: "repo", Number: 103},
	}

	results, err := client.BatchGetReviewerGroups(context.Background(), prs)
	if err != nil {
		t.Fatalf("BatchGetReviewerGroups failed: %v", err)
	}

	// Verify PR 101: Has team "backend-team" and user "other-user" in RequestedUsers
	data101, ok := results["owner/repo/101"]
	if !ok {
		t.Fatal("Result for PR 101 not found")
	}
	if len(data101.ReviewerGroups) != 1 || data101.ReviewerGroups[0] != "backend-team" {
		t.Errorf("PR 101: Expected ReviewerGroups ['backend-team'], got %v", data101.ReviewerGroups)
	}
	if len(data101.RequestedUsers) != 1 || data101.RequestedUsers[0] != "other-user" {
		t.Errorf("PR 101: Expected RequestedUsers ['other-user'], got %v", data101.RequestedUsers)
	}

	// Verify PR 102: Personal request for "test-user" -> no teams, but in RequestedUsers
	data102, ok := results["owner/repo/102"]
	if !ok {
		t.Fatal("Result for PR 102 not found")
	}
	if len(data102.ReviewerGroups) != 0 {
		t.Errorf("PR 102: Expected empty ReviewerGroups, got %v", data102.ReviewerGroups)
	}
	if len(data102.RequestedUsers) != 1 || data102.RequestedUsers[0] != "test-user" {
		t.Errorf("PR 102: Expected RequestedUsers ['test-user'], got %v", data102.RequestedUsers)
	}

	// Verify PR 103: No requests -> empty
	data103, ok := results["owner/repo/103"]
	if !ok {
		t.Fatal("Result for PR 103 not found")
	}
	if len(data103.ReviewerGroups) != 0 {
		t.Errorf("PR 103: Expected empty ReviewerGroups, got %v", data103.ReviewerGroups)
	}
	if len(data103.RequestedUsers) != 0 {
		t.Errorf("PR 103: Expected empty RequestedUsers, got %v", data103.RequestedUsers)
	}
}

func TestSearchOpenPRs_FallsBackToRESTWithoutAppClient(t *testing.T) {
	mockResponse := `{
		"total_count": 1,
		"items": [
			{
				"number": 101,
				"title": "Example PR",
				"repository_url": "https://api.github.com/repos/acme/widgets",
				"updated_at": "2026-04-15T17:00:00Z",
				"pull_request": {}
			}
		]
	}`

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.RawQuery, "q=type%3Apr+state%3Aopen+org%3Aacme") {
			t.Fatalf("unexpected query string: %s", r.URL.RawQuery)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(mockResponse))
	}))
	defer ts.Close()

	client := NewClient("dummy-token", "test-user")
	client.gh.BaseURL = mustParseURL(ts.URL + "/")
	client.gh.UploadURL = mustParseURL(ts.URL + "/")

	results, err := client.SearchOpenPRs(context.Background(), "acme")
	if err != nil {
		t.Fatalf("SearchOpenPRs failed: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Owner != "acme" || results[0].Repo != "widgets" || results[0].Number != 101 {
		t.Fatalf("unexpected result: %+v", results[0])
	}
	if results[0].UpdatedAt == nil {
		t.Fatal("expected UpdatedAt to be populated")
	}
}

func TestBatchGetPRDetails_FallsBackToRESTWithoutAppClient(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/acme/widgets/pulls/101" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"number": 101,
			"title": "Example PR",
			"html_url": "https://github.com/acme/widgets/pull/101",
			"user": { "login": "test-user" },
			"created_at": "2026-04-15T17:00:00Z",
			"draft": false,
			"head": { "sha": "abc123def456" }
		}`))
	}))
	defer ts.Close()

	client := NewClient("dummy-token", "test-user")
	client.gh.BaseURL = mustParseURL(ts.URL + "/")
	client.gh.UploadURL = mustParseURL(ts.URL + "/")

	results, err := client.BatchGetPRDetails(context.Background(), []PRInfo{
		{Owner: "acme", Repo: "widgets", Number: 101},
	})
	if err != nil {
		t.Fatalf("BatchGetPRDetails failed: %v", err)
	}

	pr, ok := results["acme/widgets/101"]
	if !ok {
		t.Fatal("expected PR details to be returned")
	}
	if pr.Title != "Example PR" || pr.Author != "test-user" || pr.CommitSHA != "abc123def456" {
		t.Fatalf("unexpected PR details: %+v", pr)
	}
}

func mustParseURL(raw string) *url.URL {
	parsed, err := url.Parse(raw)
	if err != nil {
		panic(err)
	}
	return parsed
}

// redirectTransport redirects all requests to the target URL
type redirectTransport struct {
	targetURL string
}

func (t *redirectTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Create a new request to the target URL
	newReq, err := http.NewRequest(req.Method, t.targetURL, req.Body)
	if err != nil {
		return nil, err
	}
	newReq.Header = req.Header
	return http.DefaultTransport.RoundTrip(newReq)
}

// TestBatchGetReviewerGroups_ChunksLargeBatch verifies the fix for the silent
// GraphQL truncation bug: a repo with more PRs than reviewerGroupsChunkSize must
// be split across multiple bounded queries, and every PR's reviewer group must
// come back (not just those that would have fit in one oversized query).
func TestBatchGetReviewerGroups_ChunksLargeBatch(t *testing.T) {
	prNumRe := regexp.MustCompile(`number: (\d+)`)

	var mu sync.Mutex
	var requestCount int
	maxAliasesPerRequest := 0

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		nums := prNumRe.FindAllStringSubmatch(string(body), -1)

		mu.Lock()
		requestCount++
		if len(nums) > maxAliasesPerRequest {
			maxAliasesPerRequest = len(nums)
		}
		mu.Unlock()

		// Respond with one team per PR, aliased pr0..prN in request order, so a
		// dropped/mis-merged chunk would show up as a missing result.
		var sb strings.Builder
		sb.WriteString(`{"data":{`)
		for i, m := range nums {
			if i > 0 {
				sb.WriteString(",")
			}
			fmt.Fprintf(&sb, `"pr%d":{"pullRequest":{"number":%s,"timelineItems":{"nodes":[{"requestedReviewer":{"__typename":"Team","name":"team-%s","slug":"team-%s"}}]}}}`,
				i, m[1], m[1], m[1])
		}
		sb.WriteString(`}}`)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(sb.String()))
	}))
	defer ts.Close()

	client := NewClient("dummy-token", "test-user")
	client.httpClient = &http.Client{Transport: &redirectTransport{targetURL: ts.URL}}

	const total = 60 // > 2 chunks of 25
	var prs []PullRequest
	for n := 1; n <= total; n++ {
		prs = append(prs, PullRequest{Owner: "owner", Repo: "repo", Number: n})
	}

	results, err := client.BatchGetReviewerGroups(context.Background(), prs)
	if err != nil {
		t.Fatalf("BatchGetReviewerGroups failed: %v", err)
	}

	if len(results) != total {
		t.Errorf("expected %d results, got %d (PRs lost across chunks)", total, len(results))
	}
	for n := 1; n <= total; n++ {
		key := fmt.Sprintf("owner/repo/%d", n)
		data, ok := results[key]
		if !ok {
			t.Errorf("missing result for PR %d", n)
			continue
		}
		want := fmt.Sprintf("team-%d", n)
		if len(data.ReviewerGroups) != 1 || data.ReviewerGroups[0] != want {
			t.Errorf("PR %d: expected [%s], got %v", n, want, data.ReviewerGroups)
		}
	}

	wantRequests := (total + reviewerGroupsChunkSize - 1) / reviewerGroupsChunkSize
	if requestCount != wantRequests {
		t.Errorf("expected %d chunked requests, got %d", wantRequests, requestCount)
	}
	if maxAliasesPerRequest > reviewerGroupsChunkSize {
		t.Errorf("a request carried %d PRs, exceeding chunk size %d", maxAliasesPerRequest, reviewerGroupsChunkSize)
	}
}

// A transient per-repo failure (non-rate-limit) must make BatchGetReviewerGroups
// return an error so the poller skips the reviewer-groups phase (incl. the prune)
// rather than acting on a partial result map.
func TestBatchGetReviewerGroups_PartialFetchFailureReturnsError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "bad gateway", http.StatusBadGateway)
	}))
	defer ts.Close()

	client := NewClient("dummy-token", "test-user")
	client.httpClient = &http.Client{Transport: &redirectTransport{targetURL: ts.URL}}

	_, err := client.BatchGetReviewerGroups(context.Background(), []PullRequest{
		{Owner: "owner", Repo: "repo", Number: 1},
	})
	if err == nil {
		t.Fatal("expected an error when a repo fetch fails, got nil")
	}
}

// A rate-limited response must surface as ErrGraphQLRateLimited specifically.
func TestBatchGetReviewerGroups_RateLimitReturnsTypedError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":null,"errors":[{"type":"RATE_LIMITED","message":"API rate limit already exceeded"}]}`))
	}))
	defer ts.Close()

	client := NewClient("dummy-token", "test-user")
	client.httpClient = &http.Client{Transport: &redirectTransport{targetURL: ts.URL}}

	_, err := client.BatchGetReviewerGroups(context.Background(), []PullRequest{
		{Owner: "owner", Repo: "repo", Number: 1},
	})
	if !errors.Is(err, ErrGraphQLRateLimited) {
		t.Fatalf("expected ErrGraphQLRateLimited, got %v", err)
	}
}
