package html

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"pr-review-server/pkg/reviewer/types"
)

func TestParseDiff(t *testing.T) {
	diff := `diff --git a/file1.txt b/file1.txt
index e69de29..d00491f 100644
--- a/file1.txt
+++ b/file1.txt
@@ -0,0 +1 @@
+Hello World
diff --git a/file2.go b/file2.go
index 6e9b81b..1b61971 100644
--- a/file2.go
+++ b/file2.go
@@ -1,4 +1,4 @@
 package main

-func main() {}
+func main() { /* hello */ }

`
	expected := []*DiffFile{
		{
			Path: "file1.txt",
			Lines: []Line{
				{Type: "added", Content: "Hello World", OldLineNumber: 0, NewLineNumber: 1},
			},
		},
		{
			Path: "file2.go",
			Lines: []Line{
				{Type: "context", Content: "package main", OldLineNumber: 1, NewLineNumber: 1},
				{Type: "context", Content: "", OldLineNumber: 2, NewLineNumber: 2},
				{Type: "deleted", Content: "func main() {}", OldLineNumber: 3, NewLineNumber: 0},
				{Type: "added", Content: "func main() { /* hello */ }", OldLineNumber: 0, NewLineNumber: 3},
				{Type: "context", Content: "", OldLineNumber: 4, NewLineNumber: 4},
			},
		},
	}

	actual := ParseDiff(diff)

	if !reflect.DeepEqual(actual, expected) {
		t.Errorf("ParseDiff() = %v, want %v", actual, expected)
		// For more detailed output on mismatch
		for i := range actual {
			if i >= len(expected) {
				t.Errorf("Extra file in actual: %+v", actual[i])
				continue
			}
			if !reflect.DeepEqual(actual[i], expected[i]) {
				t.Errorf("Mismatch in file %d:", i)
				t.Errorf("  Actual:   %+v", actual[i])
				t.Errorf("  Expected: %+v", expected[i])
				for j := range actual[i].Lines {
					if j >= len(expected[i].Lines) {
						t.Errorf("    Extra line in actual: %+v", actual[i].Lines[j])
						continue
					}
					if !reflect.DeepEqual(actual[i].Lines[j], expected[i].Lines[j]) {
						t.Errorf("    Mismatch in line %d:", j)
						t.Errorf("      Actual:   %+v", actual[i].Lines[j])
						t.Errorf("      Expected: %+v", expected[i].Lines[j])
					}
				}
			}
		}
	}
}

func TestGenerateReport_WithGeneralComments(t *testing.T) {
	comments := []types.LineComment{
		{
			FilePath:    "app.go",
			LineNumber:  13,
			CommentBody: "Add error handling here",
			Importance:  "CRITICAL",
		},
		{
			FilePath:    "GENERAL",
			LineNumber:  0,
			CommentBody: "Overall architecture looks good",
			Importance:  "LOW",
		},
		{
			FilePath:    "SUMMARY",
			LineNumber:  0,
			CommentBody: "## Test Coverage Analysis\nAdequate coverage overall",
			Importance:  "",
		},
	}

	diff := `diff --git a/app.go b/app.go
index 123..456 100644
--- a/app.go
+++ b/app.go
@@ -12,6 +12,7 @@ func main() {
 	fmt.Println("Hello")
+	// New line
 }`

	testTime := time.Date(2024, 1, 15, 14, 30, 0, 0, time.UTC)
	report, err := GenerateReport(comments, diff, 123, "https://github.com/test-owner/test-repo/pull/123", "Test PR body", "", "abc1234", "gemini-pro", 0, 0, 0, testTime)
	assert.NoError(t, err)

	// Should contain general comments section
	assert.Contains(t, report, "<h2>General Comments</h2>")
	assert.Contains(t, report, "Overall architecture looks good")
	assert.Contains(t, report, "class=\"general-comment\"")

	// Should contain summary section
	assert.Contains(t, report, "<h2>Review Summary</h2>")
	assert.Contains(t, report, "Test Coverage Analysis")

	// Should contain file-specific comments
	assert.Contains(t, report, "Add error handling here")

	// Should have correct comment counts - the order depends on how they're processed
	assert.Contains(t, report, "Comment 2 of 3") // General comment
	assert.Contains(t, report, "Comment 3 of 3") // Summary comment
	// Note: File-specific comment might not appear inline if line not in diff
}

func TestGenerateReport_WithAdjacentComments(t *testing.T) {
	// Comments that reference lines not in the diff should appear in adjacent section
	comments := []types.LineComment{
		{
			FilePath:    "app.go",
			LineNumber:  999, // Line not in diff
			CommentBody: "This line is not in the diff",
			Importance:  "CRITICAL",
		},
		{
			FilePath:    "test.go",
			LineNumber:  13, // Line that IS in diff
			CommentBody: "This line is in the diff",
			Importance:  "MEDIUM",
		},
		{
			FilePath:    "SUMMARY",
			LineNumber:  0,
			CommentBody: "## Testing Summary\nOverall assessment",
			Importance:  "",
		},
	}

	diff := `diff --git a/test.go b/test.go
index 123..456 100644
--- a/test.go
+++ b/test.go
@@ -10,6 +10,7 @@ func main() {
 	fmt.Println("Hello")
+	// New line at 13
 }`

	// Provide file contents to enable adjacent comments functionality
	fileContents := map[string]string{
		"app.go": `package main

import "fmt"

func main() {
    fmt.Println("Hello")
}`,
	}

	testTime := time.Date(2024, 1, 15, 14, 30, 0, 0, time.UTC)
	report, err := GenerateReportWithContext(comments, diff, 123, "https://github.com/test-owner/test-repo/pull/123", "Test PR body", "", "abc1234", "gemini-pro", 0, 0, 0, testTime, fileContents)
	assert.NoError(t, err)

	// Should contain adjacent comments section for comment that can't be displayed inline
	assert.Contains(t, report, "<h2>Adjacent File-Specific Comments</h2>")
	assert.Contains(t, report, "This line is not in the diff")
	assert.Contains(t, report, "app.go:999")
	assert.Contains(t, report, "class=\"diff-file\"") // Now uses diff-file structure

	// Should contain summary section
	assert.Contains(t, report, "<h2>Review Summary</h2>")
	assert.Contains(t, report, "Testing Summary")

	// The inline comment should appear with the diff (test.go:13)
	assert.Contains(t, report, "This line is in the diff")

	// Should have correct comment counts
	assert.Contains(t, report, "Comment 1 of 3")
	assert.Contains(t, report, "Comment 2 of 3")
	assert.Contains(t, report, "Comment 3 of 3")
}

func TestGenerateReport_WithoutAdjacentComments(t *testing.T) {
	// When fileContents is nil (adjacent comments disabled), adjacent comments should not appear
	comments := []types.LineComment{
		{
			FilePath:    "app.go",
			LineNumber:  999, // Line not in diff
			CommentBody: "This line is not in the diff",
			Importance:  "CRITICAL",
		},
		{
			FilePath:    "test.go",
			LineNumber:  13, // Line in diff
			CommentBody: "This line is in the diff",
			Importance:  "MEDIUM",
		},
		{
			FilePath:    "SUMMARY",
			LineNumber:  0,
			CommentBody: "## Testing Summary\n\nOverall assessment",
		},
	}

	diff := `diff --git a/test.go b/test.go
index 123..456 100644
--- a/test.go
+++ b/test.go
@@ -10,6 +10,7 @@ func main() {
 	fmt.Println("Hello")
+	// New line at 13
 }`

	// Use GenerateReport (no file contents) to simulate adjacent comments disabled
	testTime := time.Date(2024, 1, 15, 14, 30, 0, 0, time.UTC)
	report, err := GenerateReport(comments, diff, 123, "https://github.com/test-owner/test-repo/pull/123", "Test PR body", "", "abc1234", "gemini-pro", 0, 0, 0, testTime)
	assert.NoError(t, err)

	// Should NOT contain adjacent comments section since fileContents is nil
	assert.NotContains(t, report, "<h2>Adjacent File-Specific Comments</h2>")
	assert.NotContains(t, report, "This line is not in the diff")
	assert.NotContains(t, report, "app.go:999")

	// Should still contain summary section
	assert.Contains(t, report, "<h2>Review Summary</h2>")
	assert.Contains(t, report, "Testing Summary")
}

func TestGenerateReportWithContext_ShowsContextLines(t *testing.T) {
	// Comments that reference lines with file context available
	comments := []types.LineComment{
		{
			FilePath:    "app.go",
			LineNumber:  5, // Line not in diff but available in file content
			CommentBody: "This function needs error handling",
			Importance:  "CRITICAL",
		},
		{
			FilePath:    "SUMMARY",
			LineNumber:  0,
			CommentBody: "## Testing Summary\nOverall good",
			Importance:  "",
		},
	}

	// File contents with the referenced line
	fileContents := map[string]string{
		"app.go": `package main

import "fmt"

func hello() {
    fmt.Println("Hello, World!")
    return
}
`,
	}

	diff := `diff --git a/other.go b/other.go
index 123..456 100644
--- a/other.go  
+++ b/other.go
@@ -1,3 +1,4 @@ 
 package main
+// New line
 func main() {
 }`

	testTime := time.Date(2024, 1, 15, 14, 30, 0, 0, time.UTC)
	report, err := GenerateReportWithContext(comments, diff, 123, "https://github.com/test-owner/test-repo/pull/123", "Test PR body", "", "abc1234", "gemini-pro", 0, 0, 0, testTime, fileContents)
	assert.NoError(t, err)

	// Should contain adjacent section with context lines using diff-file structure
	assert.Contains(t, report, "<h2>Adjacent File-Specific Comments</h2>")
	assert.Contains(t, report, "app.go:5")
	assert.Contains(t, report, "class=\"diff-file\"") // Now uses diff-file structure
	assert.Contains(t, report, "target-line")         // The highlighted line
	assert.Contains(t, report, "func hello()")        // Context line content

	// Should contain summary section
	assert.Contains(t, report, "<h2>Review Summary</h2>")
	assert.Contains(t, report, "Testing Summary")
}

// TestGenerateReport_EscapesUntrustedHTML guards against HTML injection / stored
// XSS: code, diffs, PR descriptions, and AI comments come from untrusted sources
// and must never be rendered as live HTML in the review page.
func TestGenerateReport_EscapesUntrustedHTML(t *testing.T) {
	comments := []types.LineComment{
		{
			FilePath:    "evil.js",
			LineNumber:  2, // not in diff -> rendered via context lines
			CommentBody: "Look here: <img src=x onerror=COMMENT_XSS()>",
			Importance:  "CRITICAL",
		},
	}

	// Source code containing markup that must be escaped, not executed.
	fileContents := map[string]string{
		"evil.js": "function init() {\n  $('<script>CTX_XSS()</script>');\n  {% if user %}\n}\n",
	}

	diff := `diff --git a/other.go b/other.go
index 123..456 100644
--- a/other.go
+++ b/other.go
@@ -1,2 +1,3 @@
 package main
+// added
 func main() {}`

	prBody := "PR description with injected <script>BODY_XSS()</script> markup"
	prompt := "Review this code:\n<script type=\"text/javascript\">PROMPT_XSS()</script>\n$('#token_table')"

	testTime := time.Date(2024, 1, 15, 14, 30, 0, 0, time.UTC)
	report, err := GenerateReportWithContext(comments, diff, 1, "https://github.com/acme/example/pull/1", prBody, prompt, "abc1234", "gemini-pro", 0, 0, 0, testTime, fileContents)
	assert.NoError(t, err)

	// No injected payload may appear as a live element/handler anywhere.
	assert.NotContains(t, report, "<script>BODY_XSS()", "PR body script must be sanitized")
	assert.NotContains(t, report, "<script>CTX_XSS()", "context-line script must be escaped")
	assert.NotContains(t, report, "<script type=\"text/javascript\">PROMPT_XSS()", "prompt script must be escaped")
	assert.NotContains(t, report, "onerror=", "comment-body event handler must be sanitized")

	// The prompt and context code must survive as escaped, readable text.
	assert.Contains(t, report, "&lt;script", "escaped markup should still be visible")

	// And the benign parts of the untrusted input must still render, so we'd
	// catch a regression where sanitizing accidentally strips everything.
	assert.Contains(t, report, "PR description with injected", "PR body text should still render")
	assert.Contains(t, report, "{% if user %}", "context-line code should still render (escaped)")
	assert.Contains(t, report, "Look here:", "comment body text should still render")
}

func TestRenderMarkdown(t *testing.T) {
	t.Run("strips dangerous HTML", func(t *testing.T) {
		out := string(renderMarkdown("hello <script>alert(1)</script> world"))
		assert.NotContains(t, out, "<script>")
		assert.NotContains(t, out, "alert(1)")
	})

	t.Run("strips event-handler attributes", func(t *testing.T) {
		out := string(renderMarkdown("![x](data:image/png;base64,abc) <img src=x onerror=alert(1)>"))
		assert.NotContains(t, out, "onerror")
	})

	t.Run("preserves real markdown formatting", func(t *testing.T) {
		out := string(renderMarkdown("This is **bold** and _italic_."))
		assert.Contains(t, out, "<strong>bold</strong>")
		assert.Contains(t, out, "<em>italic</em>")
	})

	t.Run("renders code spans with their angle brackets escaped, not executed", func(t *testing.T) {
		out := string(renderMarkdown("use the `<div>` element"))
		assert.Contains(t, out, "<code>&lt;div&gt;</code>")
		assert.NotContains(t, out, "<div>")
	})
}

func TestGenerateContextLines_EscapesContent(t *testing.T) {
	fileContents := map[string]string{
		"x.js": "line one\n<script>alert(1)</script>\nline three\n",
	}

	lines := GenerateContextLinesForTest("x.js", 2, fileContents)
	if assert.NotEmpty(t, lines) {
		var joined string
		for _, l := range lines {
			joined += string(l.Content)
		}
		assert.Contains(t, joined, "&lt;script&gt;", "source code must be HTML-escaped")
		assert.NotContains(t, joined, "<script>", "source code must not render as a live tag")
	}
}

func TestGenerateReport_WholeFileCommentRendersOnce(t *testing.T) {
	// A whole-file finding (LineNumber 0, e.g. a mechanical gate alert) must
	// render exactly once under the file header — not once per deleted line
	// (deleted lines carry NewLineNumber 0 and used to collide with the
	// comment's line-0 key).
	comments := []types.LineComment{
		{
			FilePath:    "worker.py",
			LineNumber:  0,
			CommentBody: "Mechanical alert — task payload contract changed.",
			Importance:  "MEDIUM",
		},
	}

	diff := `diff --git a/worker.py b/worker.py
index 123..456 100644
--- a/worker.py
+++ b/worker.py
@@ -10,6 +10,3 @@ def handle():
 	keep = 1
-	removed_one = 1
-	removed_two = 2
-	removed_three = 3
 	also_keep = 2`

	testTime := time.Date(2024, 1, 15, 14, 30, 0, 0, time.UTC)
	report, err := GenerateReport(comments, diff, 123, "https://github.com/test-owner/test-repo/pull/123", "Test PR body", "", "abc1234", "gemini-pro", 0, 0, 0, testTime)
	assert.NoError(t, err)

	occurrences := strings.Count(report, "<p>Mechanical alert — task payload contract changed.</p>")
	assert.Equal(t, 1, occurrences, "whole-file comment should render exactly once, got %d", occurrences)
}

func TestGenerateReport_WholeFileCommentOnOffDiffFile(t *testing.T) {
	// A whole-file comment (LineNumber 0) whose file is NOT in the diff takes
	// the adjacent-comments path, where generateContextLines rejects line 0
	// and returns no context lines. It must still appear in the output.
	comments := []types.LineComment{
		{
			FilePath:    "other.py",
			LineNumber:  0,
			CommentBody: "Whole-file note on a file outside the diff",
			Importance:  "MEDIUM",
		},
	}

	diff := `diff --git a/app.go b/app.go
index 123..456 100644
--- a/app.go
+++ b/app.go
@@ -12,6 +12,7 @@ func main() {
 	fmt.Println("Hello")
+	// New line
 }`

	fileContents := map[string]string{"other.py": "line one\nline two\n"}
	testTime := time.Date(2024, 1, 15, 14, 30, 0, 0, time.UTC)
	report, err := GenerateReportWithContext(comments, diff, 123, "https://github.com/test-owner/test-repo/pull/123", "Test PR body", "", "abc1234", "gemini-pro", 0, 0, 0, testTime, fileContents)
	assert.NoError(t, err)

	assert.Equal(t, 1, strings.Count(report, "Whole-file note on a file outside the diff"),
		"off-diff whole-file comment should render exactly once in the adjacent section")
	assert.Contains(t, report, "adjacent-section")
}

func TestGenerateReport_CommentPermalinkButtons(t *testing.T) {
	// Every rendered comment gets a copy-link button, whatever section it lands
	// in: summary, general, inline-with-diff, whole-file, and adjacent.
	comments := []types.LineComment{
		{FilePath: "app.go", LineNumber: 13, CommentBody: "Inline finding", Importance: "HIGH"},
		{FilePath: "app.go", LineNumber: 0, CommentBody: "Whole-file finding", Importance: "MEDIUM"},
		{FilePath: "GENERAL", LineNumber: 0, CommentBody: "General finding", Importance: "LOW"},
		{FilePath: "SUMMARY", LineNumber: 0, CommentBody: "Summary finding", Importance: ""},
		{FilePath: "other.go", LineNumber: 2, CommentBody: "Adjacent finding", Importance: "LOW"},
	}

	diff := `diff --git a/app.go b/app.go
index 123..456 100644
--- a/app.go
+++ b/app.go
@@ -12,6 +12,7 @@ func main() {
 	fmt.Println("Hello")
+	// New line
 }`

	fileContents := map[string]string{"other.go": "package other\n\nfunc f() {}\n"}
	testTime := time.Date(2024, 1, 15, 14, 30, 0, 0, time.UTC)
	report, err := GenerateReportWithContext(comments, diff, 123, "https://github.com/test-owner/test-repo/pull/123", "Test PR body", "", "abc1234", "gemini-pro", 0, 0, 0, testTime, fileContents)
	assert.NoError(t, err)

	assert.Equal(t, 5, strings.Count(report, `aria-label="Copy link to this comment"`),
		"each of the five rendered comments should carry a copy-link button")

	// The label lives in its own span so the load-time renumbering can rewrite
	// the text without wiping out the button next to it.
	assert.Equal(t, 5, strings.Count(report, `class="comment-counter-text"`))

	// Runtime wiring: ids to link to, and fragment handling to act on them.
	assert.Contains(t, report, "comment.id = `comment-${number}`")
	assert.Contains(t, report, "focusCommentFromHash")
	assert.Contains(t, report, "hashchange")
}
