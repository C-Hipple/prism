package poller

import (
	"fmt"

	"pr-review-server/db"
	"pr-review-server/github"
)

// mergePRLists combines PRs from multiple sources, removing duplicates.
// PRs are identified by owner/repo/number.
func mergePRLists(reviewPRs, myPRs []github.PullRequest, dbPRs []db.PR) []github.PullRequest {
	seen := make(map[string]bool)
	var result []github.PullRequest

	// Helper to add PRs with deduplication
	addPRs := func(prs []github.PullRequest) {
		for _, pr := range prs {
			key := fmt.Sprintf("%s/%s/%d", pr.Owner, pr.Repo, pr.Number)
			if !seen[key] {
				seen[key] = true
				result = append(result, pr)
			}
		}
	}

	// Add PRs in order of priority: review requests, my PRs, then database PRs
	addPRs(reviewPRs)
	addPRs(myPRs)

	// Convert database PRs to github.PullRequest format and add
	for _, dbPR := range dbPRs {
		key := fmt.Sprintf("%s/%s/%d", dbPR.RepoOwner, dbPR.RepoName, dbPR.PRNumber)
		if !seen[key] {
			seen[key] = true
			result = append(result, buildPRFromDB(dbPR))
		}
	}

	return result
}

// groupPRsByRepo groups PRs by their repository key (owner/repo).
func groupPRsByRepo(prs []github.PullRequest) map[string][]github.PullRequest {
	prsByRepo := make(map[string][]github.PullRequest)
	for _, pr := range prs {
		repoKey := fmt.Sprintf("%s/%s", pr.Owner, pr.Repo)
		prsByRepo[repoKey] = append(prsByRepo[repoKey], pr)
	}
	return prsByRepo
}

// buildPRFromDB converts a database PR record to a github.PullRequest.
func buildPRFromDB(dbPR db.PR) github.PullRequest {
	return github.PullRequest{
		Owner:     dbPR.RepoOwner,
		Repo:      dbPR.RepoName,
		Number:    dbPR.PRNumber,
		CommitSHA: dbPR.LastCommitSHA,
		Title:     dbPR.Title,
		Author:    dbPR.Author,
		URL:       fmt.Sprintf("https://github.com/%s/%s/pull/%d", dbPR.RepoOwner, dbPR.RepoName, dbPR.PRNumber),
		CreatedAt: dbPR.CreatedAt,
		Draft:     dbPR.Draft,
	}
}

// shouldSkipDatabasePR determines if a database PR should be skipped during processing.
// A PR is skipped if auto-review is disabled and the PR is not actively generating.
func shouldSkipDatabasePR(dbPR db.PR, autoReviewEnabled bool) bool {
	// Never skip PRs that are actively generating (manual triggers)
	if dbPR.Status == "generating" {
		return false
	}

	// Skip if auto-review is disabled and PR is pending
	if !autoReviewEnabled && dbPR.Status == "pending" {
		return true
	}

	return false
}
