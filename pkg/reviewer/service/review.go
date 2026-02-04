package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/fatih/color"
	"pr-review-server/github"
	"pr-review-server/pkg/reviewer/llm"
	"pr-review-server/pkg/reviewer/types"
)

var (
	magenta = color.New(color.FgMagenta)
	yellow  = color.New(color.FgYellow)
	white   = color.New(color.FgWhite)
	blue    = color.New(color.FgBlue)
)

// GithubClient defines the interface for GitHub operations required by the reviewer
type GithubClient interface {
	GetReviewPR(token, owner, repoName string, prNumber int) (github.PR, error)
	GetPRDiff(token, owner, repoName string, prNumber int) (string, error)
	GetPRFiles(token, owner, repoName string, prNumber int) ([]github.PRFile, error)
	GetFileContent(token, owner, repoName, filePath, branch string) (string, error)
	PostLineComment(token, owner, repoName string, prNumber int, commitID, comment, path string, line int) error
	PostPRComment(token, owner, repoName string, prNumber int, comment string) error
	GetPRComments(token, owner, repoName string, prNumber int) ([]github.PRComment, error)
	GetCurrentUser(token string) (github.User, error)
}

// Service encapsulates the logic for performing a code review.
type Service struct {
	githubClient   GithubClient
	smartLlmClient llm.IClient
	fastLlmClient  llm.IClient
}

// NewService creates a new review service.
func NewService(gc GithubClient, smartc llm.IClient, fastc llm.IClient) *Service {
	return &Service{githubClient: gc, smartLlmClient: smartc, fastLlmClient: fastc}
}

// PerformReviewConfig holds the configuration for a review.
type PerformReviewConfig struct {
	Token           string
	Owner           string
	RepoName        string
	PRNumber        int
	WithComments    bool
	Verbose         bool
	Fast            bool
	Adjacent        bool   // Include adjacent comments for lines not explicitly in the diff
	GitHub          bool   // Include existing GitHub comments as context for the LLM
	ExtraContext    string // optional additional context/question supplied from CLI
	CustomPrompt    string // custom review prompt to use instead of default (overrides Testing flag)
	NRequests       int
	Testing         bool              // Use testing-focused review prompt
	SelectedContext ContextDefinition // For review-tui command
	TerminalDiff    bool              // Print diff with embedded comments to terminal
}

// ReviewResult holds the results of a review.
type ReviewResult struct {
	Comments             []types.LineComment
	Diff                 string
	PRBody               string
	Prompt               string
	PromptTokenCount     int32
	CandidatesTokenCount int32
	TotalTokenCount      int32
	FileContents         map[string]string // File contents for context generation
	// Importance counts (computed from Comments)
	CriticalCount int
	MediumCount   int
	LowCount      int
}

// ComputeImportanceCounts calculates the number of comments at each importance level.
func (r *ReviewResult) ComputeImportanceCounts() {
	r.CriticalCount = 0
	r.MediumCount = 0
	r.LowCount = 0
	for _, c := range r.Comments {
		switch strings.ToUpper(c.Importance) {
		case "CRITICAL":
			r.CriticalCount++
		case "MEDIUM":
			r.MediumCount++
		case "LOW":
			r.LowCount++
		}
	}
}

// PerformReview conducts a code review for a given pull request.
// This is the main orchestration function that coordinates data fetching,
// prompt building, review execution, and post-processing.
func (s *Service) PerformReview(cfg PerformReviewConfig) (*ReviewResult, error) {
	return s.PerformReviewWithContext(context.Background(), cfg)
}

// PerformReviewWithContext conducts a code review with context support for cancellation.
func (s *Service) PerformReviewWithContext(ctx context.Context, cfg PerformReviewConfig) (*ReviewResult, error) {
	if cfg.NRequests == 0 {
		cfg.NRequests = 1
	}

	prefix := prLogPrefix(cfg.Owner, cfg.RepoName, cfg.PRNumber)

	// Phase 1: Fetch all PR data
	data, err := s.fetchPRData(ctx, cfg)
	if err != nil {
		return nil, err
	}

	// Phase 2: Build the prompt
	fullPrompt := s.buildPrompt(cfg, data)

	if cfg.Verbose {
		blue.Printf("%s --- LLM PROMPT ---\n", prefix)
		white.Println(fullPrompt)
		blue.Printf("%s --- END LLM PROMPT ---\n", prefix)
	}

	// Phase 3: Execute review requests
	color.White("%s Sending to AI for review...", prefix)
	var execResult *ReviewExecutionResult
	var allComments []types.LineComment

	if cfg.CustomPrompt != "" {
		// Custom prompts use plain text format
		execResult, err = s.executeCustomPromptReview(cfg, fullPrompt)
		if err != nil {
			return nil, err
		}
		allComments = execResult.Comments
	} else {
		// Standard JSON-based review flow
		execResult = s.executeStandardReview(ctx, cfg, fullPrompt)
		allComments = execResult.Comments

		// Handle empty results
		if len(allComments) == 0 && cfg.WithComments {
			color.Yellow("%s No comments were generated from the AI review, posting a general comment.", prefix)
			_ = s.githubClient.PostPRComment(cfg.Token, cfg.Owner, cfg.RepoName, cfg.PRNumber, "AI review completed without finding any specific issues to comment on.")
		}
	}

	// Phase 4: Post-process comments (classification, previous comments, summary)
	finalComments, err := s.handlePostReviewProcessing(ctx, cfg, data, allComments)
	if err != nil {
		color.Yellow("%s Warning during post-processing: %v", prefix, err)
	}

	// Phase 5: Output and post comments
	if cfg.TerminalDiff {
		PrintDiffWithComments(data.Diff, finalComments)
	} else {
		s.printComments(finalComments)
	}

	if cfg.WithComments {
		// Only post the new comments, not the previous ones (they're already posted)
		s.postComments(cfg, data.PR.Head.SHA, allComments)
	}

	// Phase 6: Validate results and build response
	if execResult.SuccessCount == 0 && cfg.NRequests > 0 && cfg.CustomPrompt == "" {
		if execResult.FirstError != nil {
			return nil, fmt.Errorf("all %d review requests failed: %w", cfg.NRequests, execResult.FirstError)
		}
		return nil, fmt.Errorf("all %d review requests failed with unknown errors", cfg.NRequests)
	}

	if execResult.SuccessCount > 0 && execResult.SuccessCount < int64(cfg.NRequests) {
		color.Yellow("%s Warning: Only %d of %d review requests succeeded", prefix, execResult.SuccessCount, cfg.NRequests)
	}

	result := &ReviewResult{
		Comments:             finalComments,
		Diff:                 data.Diff,
		PRBody:               data.PR.Body,
		Prompt:               fullPrompt,
		PromptTokenCount:     execResult.PromptTokenCount,
		CandidatesTokenCount: execResult.CandidatesTokenCount,
		TotalTokenCount:      execResult.TotalTokenCount,
		FileContents:         data.FileContents,
	}
	result.ComputeImportanceCounts()
	return result, nil
}

func (s *Service) getPRContext(token, owner, repoName string, pr github.PR, prNumber int, testing bool, includeFileContents bool, selectedContext ContextDefinition) (string, map[string]string, error) {
	prefix := prLogPrefix(owner, repoName, prNumber)
	files, err := s.githubClient.GetPRFiles(token, owner, repoName, prNumber)
	if err != nil {
		return "", nil, fmt.Errorf("error fetching PR files: %w", err)
	}

	var contextBuilder strings.Builder
	processedFiles := make(map[string]bool) // Track files we've already processed
	var fileContents map[string]string      // Store file contents for HTML context generation (only if requested)
	if includeFileContents {
		fileContents = make(map[string]string)
	}

	for _, file := range files {
		if file.Status == "added" {
			// Skip newly added files - no existing context to fetch
			continue
		}

		// Process the main file
		if !processedFiles[file.Filename] {
			content, err := s.githubClient.GetFileContent(token, owner, repoName, file.Filename, pr.Base.Ref)
			if err != nil {
				color.Yellow("%s Could not fetch file content for %s: %v", prefix, file.Filename, err)
			} else {
				contextBuilder.WriteString(fmt.Sprintf("--- FILE: %s ---\n", file.Filename))
				contextBuilder.WriteString(content)
				contextBuilder.WriteString("\n\n")
				processedFiles[file.Filename] = true

				// Store file content for HTML context generation (if requested)
				if includeFileContents {
					fileContents[file.Filename] = content
				}
			}
		}

		// Only fetch test files if in testing mode
		if testing {
			// Try to fetch corresponding test file(s) using static analysis first
			var testFilePaths []string

			// First, try static analysis to find test files that actually reference this source file
			staticAnalysisFiles := s.findTestFilesByStaticAnalysis(token, owner, repoName, file.Filename, pr.Base.Ref)
			if len(staticAnalysisFiles) > 0 {
				testFilePaths = staticAnalysisFiles
				color.White("%s Detected %d potential test file(s) via static analysis for %s", prefix, len(staticAnalysisFiles), file.Filename)
			} else {
				// Fallback to pattern-based matching
				testFilePaths = s.getTestFilePaths(file.Filename)
			}

			testFileFound := false

			for _, testFilePath := range testFilePaths {
				if testFilePath != "" && !processedFiles[testFilePath] {
					testContent, err := s.githubClient.GetFileContent(token, owner, repoName, testFilePath, pr.Base.Ref)
					if err != nil {
						// Continue trying other patterns
						continue
					} else {
						contextBuilder.WriteString(fmt.Sprintf("--- TEST FILE: %s ---\n", testFilePath))
						contextBuilder.WriteString(testContent)
						contextBuilder.WriteString("\n\n")
						processedFiles[testFilePath] = true

						// Store test file content for HTML context generation (if requested)
						if includeFileContents {
							fileContents[testFilePath] = testContent
						}

						color.White("%s Found and included test file: %s", prefix, testFilePath)
						testFileFound = true
						break // Found one, stop looking
					}
				}
			}

			if !testFileFound && len(testFilePaths) > 0 {
				color.White("%s No corresponding test file found for %s (tried %d patterns)", prefix, file.Filename, len(testFilePaths))
			}
		}
	}

	// Add selected context for review-tui command
	if selectedContext.Name != "Default" {
		color.White("%s Fetching related context for %s...", prefix, selectedContext.Name)
		contextBuilder.WriteString(fmt.Sprintf("--- RELATED CONTEXT: %s ---\n", selectedContext.Name))
		contextBuilder.WriteString(selectedContext.Description)
		for _, file := range selectedContext.Files {
			content, err := s.githubClient.GetFileContent(token, owner, repoName, file, pr.Base.Ref)
			if err != nil {
				color.Yellow("%s Could not fetch file content for %s: %v", prefix, file, err)
				continue
			}
			contextBuilder.WriteString(fmt.Sprintf("--- RELATED CONTEXT FILE: %s ---\n", file))
			contextBuilder.WriteString(content)
			contextBuilder.WriteString(fmt.Sprintf("--- END RELATED CONTEXT FILE: %s ---\n", file))
			contextBuilder.WriteString("\n\n")

			// Store context file content for HTML context generation (if requested)
			if includeFileContents {
				fileContents[file] = content
			}
		}
		color.Green("%s Successfully fetched related context.", prefix)
	}

	return contextBuilder.String(), fileContents, nil
}

// buildPreviousCommentsContext creates a formatted string of existing comments for LLM context
func (s *Service) buildPreviousCommentsContext(comments []github.PRComment, currentUserLogin string) string {
	if len(comments) == 0 {
		return "No previous comments found on this pull request."
	}

	// Separate tool's own comments from other reviewers
	var toolComments []github.PRComment
	var otherComments []github.PRComment

	for _, comment := range comments {
		if comment.User.Login == currentUserLogin {
			toolComments = append(toolComments, comment)
		} else {
			otherComments = append(otherComments, comment)
		}
	}

	var contextBuilder strings.Builder
	contextBuilder.WriteString(fmt.Sprintf("=== EXISTING COMMENTS (%d total) ===\n", len(comments)))

	// Include other reviewers' comments first
	if len(otherComments) > 0 {
		contextBuilder.WriteString(fmt.Sprintf("\n--- OTHER REVIEWERS' COMMENTS (%d) ---\n", len(otherComments)))
		contextBuilder.WriteString("The following comments have been made by other reviewers. Avoid duplicating similar feedback:\n\n")

		for i, comment := range otherComments {
			contextBuilder.WriteString(fmt.Sprintf("Comment %d by %s (%s):\n", i+1, comment.User.Login, comment.CreatedAt.Format("2006-01-02 15:04")))

			// For line-specific comments, include location information
			if comment.Path != "" && comment.Line > 0 {
				contextBuilder.WriteString(fmt.Sprintf("  Location: %s:%d\n", comment.Path, comment.Line))
			}

			// Include the comment body with proper indentation
			commentLines := strings.Split(comment.Body, "\n")
			for _, line := range commentLines {
				contextBuilder.WriteString(fmt.Sprintf("  %s\n", line))
			}
			contextBuilder.WriteString("\n")
		}
	}

	// Include tool's own previous comments for awareness
	if len(toolComments) > 0 {
		contextBuilder.WriteString(fmt.Sprintf("\n--- YOUR PREVIOUS COMMENTS (%d) ---\n", len(toolComments)))
		contextBuilder.WriteString("The following are your own previous AI-generated comments on this PR. Avoid repeating the same observations:\n\n")

		for i, comment := range toolComments {
			contextBuilder.WriteString(fmt.Sprintf("Your comment %d (%s):\n", i+1, comment.CreatedAt.Format("2006-01-02 15:04")))

			// For line-specific comments, include location information
			if comment.Path != "" && comment.Line > 0 {
				contextBuilder.WriteString(fmt.Sprintf("  Location: %s:%d\n", comment.Path, comment.Line))
			}

			// Include the comment body with proper indentation
			commentLines := strings.Split(comment.Body, "\n")
			for _, line := range commentLines {
				contextBuilder.WriteString(fmt.Sprintf("  %s\n", line))
			}
			contextBuilder.WriteString("\n")
		}
	}

	contextBuilder.WriteString("=== END EXISTING COMMENTS ===\n")
	return contextBuilder.String()
}

// convertPreviousCommentsToLineComments converts the tool's previous GitHub comments to LineComment format
func (s *Service) convertPreviousCommentsToLineComments(existingComments []github.PRComment, currentUserLogin string) []types.LineComment {
	var toolComments []types.LineComment

	for _, comment := range existingComments {
		// Only include comments made by the current user (the tool)
		if comment.User.Login == currentUserLogin {
			lineComment := types.LineComment{
				FilePath:    comment.Path,
				LineNumber:  comment.Line,
				CommentBody: comment.Body,
			}

			// If it's a general PR comment (not line-specific), use a special marker
			if comment.Path == "" && comment.Line == 0 {
				lineComment.FilePath = "GENERAL"
				lineComment.LineNumber = 0
			}

			toolComments = append(toolComments, lineComment)
		}
	}

	return toolComments
}

// generateComprehensiveSummary creates a comprehensive summary of all review comments
func (s *Service) generateComprehensiveSummary(allComments []types.LineComment, prBody, diff string) (string, error) {
	description := github.ParsePullRequestDescription(prBody)

	// Build a formatted list of all tool-generated comments (excludes human reviewer comments)
	var commentsBuilder strings.Builder
	commentsBuilder.WriteString("=== ALL AI-GENERATED REVIEW COMMENTS ===\n")

	for i, comment := range allComments {
		commentsBuilder.WriteString(fmt.Sprintf("Comment %d:\n", i+1))
		if comment.FilePath != "" && comment.LineNumber > 0 {
			commentsBuilder.WriteString(fmt.Sprintf("  File: %s:%d\n", comment.FilePath, comment.LineNumber))
		} else if comment.FilePath == "GENERAL" {
			commentsBuilder.WriteString("  Type: General PR Comment\n")
		}
		commentsBuilder.WriteString(fmt.Sprintf("  Content: %s\n", comment.CommentBody))
		if comment.Importance != "" {
			commentsBuilder.WriteString(fmt.Sprintf("  Importance: %s\n", comment.Importance))
		}
		commentsBuilder.WriteString("\n")
	}
	commentsBuilder.WriteString("=== END COMMENTS ===\n")

	summaryPrompt := `You are a principal-level full‑stack engineer (Django + React) providing an executive testing summary based on all review feedback. Your tone should be confident and empathetic. The goal is to highlight and teach, not to scare. However, do not attempt to appear human. Do not use words like "I", "my", or "our" in your responses. 

**Your Task:**
Analyze ALL of the AI-generated review comments provided below and create a comprehensive testing-focused summary that evaluates:
- Overall testing coverage and strategy across the changes based on AI feedback
- Critical testing gaps that pose production risks as identified by the AI
- Testing recommendations and next steps derived from AI analysis

**Testing Summary Structure:**
Provide a well-structured testing assessment that includes:

1. **Test Coverage Analysis**
   - Current testing status across the test pyramid (static → unit → integration). E2E tests are not required.
   - Specific coverage gaps identified in the comments
   - Areas where existing tests may be insufficient

2. **Critical Testing Issues** (if any)
   - Must-fix testing gaps that block safe merging
   - Missing tests for critical paths, error scenarios, edge cases
   - Security, performance, or reliability testing concerns

3. **Testing Recommendations** 
   - Specific tests that should be added (with examples)
   - Testing strategy improvements
   - Tools, frameworks, or approaches to consider

4. **Test Quality Concerns**
   - Flaky tests, brittle fixtures, or poor test design
   - Missing test data management or determinism issues
   - Integration and deployment testing gaps

5. **Next Steps for Testing**
   - Priority order for addressing testing gaps
   - Estimated effort and impact
   - Whether additional testing review cycles are needed

**Context & Standards:**
- Backend: Python <3.12> | Django/DRF <5.x/3.x> | pytest-django
- Frontend: React <18.x/19.x> | Next.js | TypeScript | Vitest/Jest + RTL

**Guidelines:**
- Be concise but comprehensive about testing aspects
- Focus exclusively on testing-related insights from all comments
- Group testing feedback by risk level and test type
- Highlight the most critical testing gaps

**Output Format:**
Provide your summary as plain text (not JSON). Structure it clearly with headers and bullet points for testing readability.`

	var promptBuilder strings.Builder
	promptBuilder.WriteString(summaryPrompt)
	promptBuilder.WriteString("\n\n--- PR CONTEXT ---\n")
	if description.Summary != "" {
		promptBuilder.WriteString(fmt.Sprintf("Summary: %s\n", description.Summary))
	}
	if description.CodeChanges != "" {
		promptBuilder.WriteString(fmt.Sprintf("Code Changes: %s\n", description.CodeChanges))
	}

	fullPrompt := fmt.Sprintf("%s\n\n--- ALL AI-GENERATED REVIEW COMMENTS ---\n%s\n\n--- DIFF ---\n%s",
		promptBuilder.String(), commentsBuilder.String(), diff)

	// Use the fast LLM for summary generation
	summary, _, _, _, err := s.fastLlmClient.GetReview(fullPrompt)
	if err != nil {
		return "", fmt.Errorf("error generating comprehensive summary: %w", err)
	}

	return summary, nil
}

func (s *Service) buildReviewPrompt(prBody, extraContext, fileContext, diff, previousCommentsContext string) string {
	description := github.ParsePullRequestDescription(prBody)
	prompt := `You are a staff engineer providing a thorough and constructive code review for a pull request.

**Your Task:**
Review the provided pull request. Analyze the diff and the full file context for any modified files.

**Review Guidelines:**
Focus on the following aspects:
- **Correctness:** Identify bugs, logical errors, or potential edge cases.
- **Readability & Maintainability:** Check for clean code, clear naming, and adherence to best practices.
- **Performance:** Point out potential performance bottlenecks.
- **Security:** Look for security vulnerabilities.
- **Test Coverage:** Ensure that new code is adequately tested.
- **Documentation:** Check for clear comments and documentation where necessary.

**AVOID DUPLICATING EXISTING FEEDBACK:**
Based on the existing comments shown below, do not repeat similar issues or suggestions that have already been raised by other reviewers. Focus on new, unique issues that haven't been covered.

**Output Format:**
- Provide your feedback as a single JSON array of review comment objects.
- Each object must have these fields:
  - "file_path": (string) The path to the file being commented on.
  - "line_number": (integer) The line number in the diff to comment on.
  - "comment_body": (string) Your review comment.
- For code suggestions, use GitHub's suggestion markdown inside 'comment_body':
` + "```suggestion\n[your suggestion]\n```" + `
- If you have no comments, return an empty JSON array: [].
- **IMPORTANT:** Do not include any explanatory text, markdown formatting, or anything else outside of the JSON array. Your entire response must be only the JSON data.`

	var promptBuilder strings.Builder
	promptBuilder.WriteString(prompt)
	promptBuilder.WriteString("\n\n--- PR CONTEXT ---\n")
	if description.Summary != "" {
		promptBuilder.WriteString(fmt.Sprintf("Summary: %s\n", description.Summary))
	}
	if description.CodeChanges != "" {
		promptBuilder.WriteString(fmt.Sprintf("Code Changes: %s\n", description.CodeChanges))
	}
	if extraContext != "" {
		promptBuilder.WriteString(fmt.Sprintf("Extra Context: %s\n", extraContext))
	}

	return fmt.Sprintf("%s\n\n--- PREVIOUS COMMENTS ---\n%s\n\n--- FILE CONTEXT ---\n%s\n\n--- DIFF ---\n%s", promptBuilder.String(), previousCommentsContext, fileContext, diff)
}

func (s *Service) buildTestingReviewPrompt(prBody, extraContext, fileContext, diff, previousCommentsContext string) string {
	description := github.ParsePullRequestDescription(prBody)
	prompt := `You are a principal-level full‑stack engineer (Django + React) performing a rigorous review focused exclusively on quality test coverage. Your tone should be confident and empathetic. The goal is to highlight and teach, not to scare. However, do not attempt to appear human. Do not use words like "I", "my", or "our" in your responses.

════════════════════
0) OBJECTIVE 
════════════════════
Your goal is to evaluate and strengthen the TESTS and TESTING STRATEGY:
• Verify that existing tests truly guard the changed behavior AND the surrounding code it interacts with.
• Identify missing tests across the pyramid/trophy (static → unit → integration) and propose concrete additions. E2E tests are not required.
• Ensure safety under real-world conditions: performance, security, accessibility, migrations, concurrency, rollouts, and recovery.
• Prefer measurable signals (coverage, mutation, flake rate, budgets) over stylistic concerns.

════════════════════
1) CONTEXT 
════════════════════
Back end
- Python: <3.12> | Django/DRF: <5.x / 3.x> | ASGI/Channels: <yes>
Front end
- React: <18.x/19.x> | Framework: <Next.js> | Type system: <TypeScript/JS>
Standards & tools
- Python: Black/isort, flake8/pylint, mypy+django‑stubs, bandit, safety
- JS: ESLint (airbnb + react‑hooks) + Prettier, typescript‑eslint, SonarJS/depcheck, npm‑audit/OWASP

════════════════════
2) SCOPE
════════════════════
• List directly touched units (Django apps/models/views/serializers/migrations/tasks, React components/hooks/pages, API clients).  
• **Blast radius map**: upstream/downstream neighbors (imports, reverse deps, DB tables, caches, feature flags, endpoints, background jobs, websockets).  
• Important areas to consider: migrations, auth, payments, PII, real‑time, heavy traffic paths.

════════════════════
3) TESTING DEEP‑DIVE (evaluate and propose)
════════════════════
For each layer, cite lines/examples and state whether tests are **Adequate / Missing / Potentially Flaky**. If missing, propose a specific test (name, level, scope, and an example scaffold).


Unit tests  
- Python (pytest‑django): services, managers, validators, serializers, custom QuerySets.  
- JS (Vitest/Jest + React Testing Library): component logic, hooks (effect deps, memoization), reducers/selectors.  
- Property‑based fuzzing (Hypothesis/Schemathesis for OpenAPI, fast‑check for JS).

Integration tests  
- Django: view/DRF endpoint + DB (transactions, constraints), Celery tasks (idempotency, retries), Channels (ws handshake/back‑pressure).  
- React: component + data layer (MSW interceptors), router transitions, forms, error boundaries, RSC/server actions.

Contract tests  
- OpenAPI diff: only additive or documented breaking changes; Schemathesis fuzz; JSON‑schema validation.  
- GraphQL: schema diff, resolver auth, DataLoader batching; Pact consumer/provider where applicable.

Performance & scalability tests  
- API latency & throughput (Locust/k6); DB query count; cache hit ratios.  
- Web: Lighthouse budgets, image optimization, code‑splitting coverage, re‑render counts for hot components.

Security tests  
- SAST/DAST clean; input validation; XSS/SQLi/SSRF checks; secret handling; CSP/Trusted Types; authZ rules.  
- Dependency vulns triaged (npm‑audit/safety/Snyk).

Reliability, resilience & rollout tests  
- Timeouts, retries, circuit breakers; chaos/latency injection; feature‑flag on/off; canary/rollback scripts.  
- **Post‑deploy smoke tests** and health checks.

Migrations & data safety tests  
- **Zero‑downtime** expand‑and‑contract; reversible; staged dry‑run with prod‑like data; backfill jobs; long‑running migration rehearsal.

Observability‑aware tests  
- Assert key logs/metrics/traces & correlation IDs on success/failure; error budgets unaffected.  
- **Continuous‑profiling** dashboards show no new hotspots under load.

Test data & determinism  
- Factory patterns over brittle fixtures; seed control; time freezing; isolation across workers; network/mock stability (VCR/pytest‑recording/MSW).  
- Flake triage: recent failures, retries, order‑dependence, global mutable state.

Surrounding code (“test the neighborhood”)  
- Identify adjacent modules most likely to break and propose *additional* tests (not touched in the diff) to guard them.  
- Examples: shared validators, auth middleware, caching layers, pagination, serialization boundaries, analytics/telemetry, error boundaries.

════════════════════
4) FINDINGS: TEST IMPACT & GAPS
════════════════════
Format per item:
Line <n> (file) — <Issue>  
Rationale: <why it matters (user impact/ops impact/regression history/budget breach)>  
Action: <Must include a specific new/updated test(s) with an example scaffold>  
Notes: <tooling refs: pytest‑parametrize, MSW, Schemathesis, Pact, Percy, axe, k6, Locust, Hypothesis, Stryker, mutmut>

════════════════════
5) METRICS & TRENDS
════════════════════
Report or request:
- Coverage Goals: MC/DC 80%, Branch/Stmt 80% (changed files & overall)  


════════════════════
6) SUMMARY & RECOMMENDATION
════════════════════
- Verdict: **Approve / Approve with suggestions / Request changes (testing)**  
- Net Coverage Metrics change: MC/DC and Branch/Stmt coverage


**Output Requirements:**
- Provide your feedback as a single JSON array of review comment objects.
- Each object must have these fields:
  - "file_path": (string) The path to the file being commented on.
  - "line_number": (integer) The line number in the diff to comment on.
  - "comment_body": (string) Your review comment.
- For code suggestions, use GitHub's suggestion markdown inside 'comment_body':
` + "```suggestion\n[your suggestion]\n```" + `
- If you have no comments, return an empty JSON array: [].

**AVOID DUPLICATING EXISTING FEEDBACK:**
Based on the existing comments shown below, do not repeat similar issues or suggestions that have already been raised by other reviewers. Focus on new, unique testing gaps and strategies that haven't been covered.

**CRITICAL DEDUPLICATION RULES:**
1. **NO DUPLICATE COMMENTS**: Each comment must address a unique issue at a unique location
2. **ONE COMMENT PER LINE**: Maximum one comment per file/line_number combination
3. **UNIQUE ISSUES ONLY**: Do not repeat the same issue across different files or lines
4. **CONSOLIDATE SIMILAR**: If multiple similar issues exist, consolidate into one comprehensive comment
5. **NO REPETITIVE VERIFICATION**: Do not create multiple comments that verify the same thing
6. **FOCUS ON SPECIFICS**: Provide specific, actionable feedback rather than general observations
7. **AVOID REDUNDANT CONTENT**: Do not repeat the same analysis, recommendations, or text in multiple comments

**IMPORTANT:** Do not include any explanatory text, markdown formatting, or anything else outside of the JSON array. Your entire response must be only the JSON data.`

	var promptBuilder strings.Builder
	promptBuilder.WriteString(prompt)
	promptBuilder.WriteString("\n\n--- PR CONTEXT ---\n")
	if description.Summary != "" {
		promptBuilder.WriteString(fmt.Sprintf("Summary: %s\n", description.Summary))
	}
	if description.CodeChanges != "" {
		promptBuilder.WriteString(fmt.Sprintf("Code Changes: %s\n", description.CodeChanges))
	}
	if extraContext != "" {
		promptBuilder.WriteString(fmt.Sprintf("Extra Context: %s\n", extraContext))
	}

	return fmt.Sprintf("%s\n\n--- PREVIOUS COMMENTS ---\n%s\n\n--- FILE CONTEXT ---\n%s\n\n--- DIFF ---\n%s", promptBuilder.String(), previousCommentsContext, fileContext, diff)
}

func (s *Service) buildCustomReviewPrompt(customPrompt, prBody, extraContext, fileContext, diff, previousCommentsContext string) string {
	description := github.ParsePullRequestDescription(prBody)

	var promptBuilder strings.Builder
	promptBuilder.WriteString(customPrompt)
	promptBuilder.WriteString("\n\n**Output Format:**\n")
	promptBuilder.WriteString("Provide your response as plain text (not JSON). Structure it clearly with headers and bullet points for readability.\n")
	promptBuilder.WriteString("\n--- PR CONTEXT ---\n")
	if description.Summary != "" {
		promptBuilder.WriteString(fmt.Sprintf("Summary: %s\n", description.Summary))
	}
	if description.CodeChanges != "" {
		promptBuilder.WriteString(fmt.Sprintf("Code Changes: %s\n", description.CodeChanges))
	}
	if extraContext != "" {
		promptBuilder.WriteString(fmt.Sprintf("Extra Context: %s\n", extraContext))
	}

	return fmt.Sprintf("%s\n\n--- PREVIOUS COMMENTS ---\n%s\n\n--- FILE CONTEXT ---\n%s\n\n--- DIFF ---\n%s", promptBuilder.String(), previousCommentsContext, fileContext, diff)
}

func (s *Service) parseAIResponse(reviewContent string) ([]types.LineComment, error) {
	// First, try JSON parsing
	trimmedReview := strings.TrimPrefix(reviewContent, "```json\n")
	trimmedReview = strings.TrimSuffix(trimmedReview, "\n```")

	var comments []types.LineComment
	err := json.Unmarshal([]byte(trimmedReview), &comments)
	if err == nil {
		return comments, nil
	}

	// If JSON parsing fails, try to parse markdown format as fallback
	markdownComments, markdownErr := s.ParseMarkdownResponse(reviewContent)
	if markdownErr == nil {
		color.Yellow("JSON parsing failed, but successfully parsed markdown format with %d comments.", len(markdownComments))
		return markdownComments, nil
	}

	// If both parsing methods fail, return the original JSON error
	return nil, fmt.Errorf("failed to parse JSON (%v) and markdown (%v)", err, markdownErr)
}

// ParseMarkdownResponse parses AI responses in markdown format as a fallback (exported for testing)
func (s *Service) ParseMarkdownResponse(content string) ([]types.LineComment, error) {
	var comments []types.LineComment
	lines := strings.Split(content, "\n")

	var currentComment *types.LineComment
	var commentBodyLines []string

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Match "File: path/to/file.ext:123" pattern
		if strings.HasPrefix(line, "File: ") {
			// Save previous comment if exists
			if currentComment != nil {
				currentComment.CommentBody = strings.TrimSpace(strings.Join(commentBodyLines, "\n"))
				if currentComment.CommentBody != "" {
					comments = append(comments, *currentComment)
				}
			}

			// Parse new comment
			fileLine := strings.TrimPrefix(line, "File: ")
			if strings.Contains(fileLine, ":") {
				parts := strings.Split(fileLine, ":")
				if len(parts) >= 2 {
					currentComment = &types.LineComment{
						FilePath: strings.TrimSpace(parts[0]),
					}

					// Parse line number
					if lineNum, err := strconv.Atoi(strings.TrimSpace(parts[1])); err == nil {
						currentComment.LineNumber = lineNum
					}

					commentBodyLines = []string{}
					continue
				}
			}
		}

		// Skip severity/rationale headers but include the content
		if strings.HasPrefix(line, "[Rationale:") ||
			strings.HasPrefix(line, "Action:") ||
			strings.HasPrefix(line, "**Issue:**") ||
			strings.HasPrefix(line, "**Rationale:**") ||
			strings.HasPrefix(line, "**Action:**") ||
			line == "" {
			if currentComment != nil && line != "" {
				commentBodyLines = append(commentBodyLines, line)
			}
			continue
		}

		// Collect comment body lines
		if currentComment != nil {
			commentBodyLines = append(commentBodyLines, line)
		}
	}

	// Save the last comment
	if currentComment != nil {
		currentComment.CommentBody = strings.TrimSpace(strings.Join(commentBodyLines, "\n"))
		if currentComment.CommentBody != "" {
			comments = append(comments, *currentComment)
		}
	}

	if len(comments) == 0 {
		return nil, fmt.Errorf("no valid comments found in markdown format")
	}

	return comments, nil
}

func (s *Service) printComments(comments []types.LineComment) {
	for _, comment := range comments {
		magenta.Printf("File: %s", comment.FilePath)
		yellow.Printf(":%d\n", comment.LineNumber)
		white.Println(comment.CommentBody)
		fmt.Println()
	}
}

func (s *Service) postComments(cfg PerformReviewConfig, headSHA string, comments []types.LineComment) {
	prefix := prLogPrefix(cfg.Owner, cfg.RepoName, cfg.PRNumber)
	color.White("%s Posting line-specific comments...", prefix)
	for _, comment := range comments {
		color.White("%s Posting comment on %s:%d", prefix, comment.FilePath, comment.LineNumber)
		err := s.githubClient.PostLineComment(cfg.Token, cfg.Owner, cfg.RepoName, cfg.PRNumber, headSHA, comment.CommentBody, comment.FilePath, comment.LineNumber)
		if err != nil {
			color.Yellow("%s Could not post comment on %s:%d: %v", prefix, comment.FilePath, comment.LineNumber, err)
		}
	}
	color.Green("%s Finished posting all comments.", prefix)
}

func (s *Service) classifyCommentImportance(cfg PerformReviewConfig, comments []types.LineComment, prBody, fileContext, diff string) ([]types.LineComment, error) {
	prefix := prLogPrefix(cfg.Owner, cfg.RepoName, cfg.PRNumber)
	color.White("%s Classifying comment importance...", prefix)

	var indexedCommentsBuilder strings.Builder
	for i, comment := range comments {
		indexedCommentsBuilder.WriteString(fmt.Sprintf("RC-%d: File: %s, Line: %d, Comment: %s\n", i+1, comment.FilePath, comment.LineNumber, comment.CommentBody))
	}

	classificationPrompt := `You are an expert software engineer tasked with classifying the importance of code review comments.
Review the provided pull request context, diff, and the list of review comments (indexed with RC-n).
Classify each comment's importance as LOW, MEDIUM, or CRITICAL.
- LOW: Optional suggestions, style nits, or minor improvements.
- MEDIUM: Important suggestions that should be addressed, but are not blockers.
- CRITICAL: Bugs, security vulnerabilities, or major design flaws that must be fixed.

Return a JSON object where keys are the comment indexes (e.g., "RC-1") and values are the importance level.
Example: {"RC-1": "MEDIUM", "RC-2": "CRITICAL"}
Do not include any other text in your response.

--- PR CONTEXT ---
%s

--- FILE CONTEXT ---
%s

--- DIFF ---
%s

--- REVIEW COMMENTS ---
%s
`
	fullPrompt := fmt.Sprintf(classificationPrompt, prBody, fileContext, diff, indexedCommentsBuilder.String())

	if cfg.Verbose {
		blue.Println("--- CLASSIFICATION PROMPT ---")
		white.Println(fullPrompt)
		blue.Println("--- END CLASSIFICATION PROMPT ---")
	}

	response, _, _, _, err := s.fastLlmClient.GetReview(fullPrompt)
	if err != nil {
		return nil, fmt.Errorf("error getting importance classification: %w", err)
	}

	var classifications map[string]string
	trimmedResponse := strings.TrimPrefix(response, "```json\n")
	trimmedResponse = strings.TrimSuffix(trimmedResponse, "\n```")
	if err := json.Unmarshal([]byte(trimmedResponse), &classifications); err != nil {
		// Truncate response for log readability
		responsePreview := response
		if len(responsePreview) > 200 {
			responsePreview = responsePreview[:200] + "..."
		}
		color.Yellow("%s Could not parse importance classifications: %v. Response preview: %s", prefix, err, responsePreview)
		// Return original comments without importance
		return comments, nil
	}

	for i := range comments {
		commentID := fmt.Sprintf("RC-%d", i+1)
		if importance, ok := classifications[commentID]; ok {
			comments[i].Importance = importance
		}
	}

	color.Green("%s Successfully classified comment importance.", prefix)
	return comments, nil
}
