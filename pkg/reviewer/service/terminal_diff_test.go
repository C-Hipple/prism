package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"pr-review-server/pkg/reviewer/types"
)

func TestParseDiffForTerminal(t *testing.T) {
	diff := `diff --git a/main.go b/main.go
index 1234567..abcdefg 100644
--- a/main.go
+++ b/main.go
@@ -1,5 +1,6 @@
 package main

+import "fmt"
+
 func main() {
-	println("hello")
+	fmt.Println("hello")
 }`

	files := parseDiffForTerminal(diff)

	assert.Len(t, files, 1, "Should parse one file")
	assert.Equal(t, "main.go", files[0].Path, "Should parse file path correctly")
	assert.Greater(t, len(files[0].Lines), 0, "Should have lines in the diff")

	// Check for added lines
	var addedLines []DiffLine
	for _, line := range files[0].Lines {
		if line.Type == "added" {
			addedLines = append(addedLines, line)
		}
	}
	assert.Greater(t, len(addedLines), 0, "Should have added lines")
}

func TestParseDiffForTerminal_MultipleFiles(t *testing.T) {
	diff := `diff --git a/file1.go b/file1.go
index 1234567..abcdefg 100644
--- a/file1.go
+++ b/file1.go
@@ -1,3 +1,4 @@
 package main
+// New comment

 func main() {
diff --git a/file2.go b/file2.go
index 2345678..bcdefgh 100644
--- a/file2.go
+++ b/file2.go
@@ -1,2 +1,3 @@
 package util
+// Another comment
 `

	files := parseDiffForTerminal(diff)

	assert.Len(t, files, 2, "Should parse two files")
	assert.Equal(t, "file1.go", files[0].Path)
	assert.Equal(t, "file2.go", files[1].Path)
}

func TestGroupCommentsByFile(t *testing.T) {
	comments := []types.LineComment{
		{FilePath: "main.go", LineNumber: 5, CommentBody: "Comment 1"},
		{FilePath: "main.go", LineNumber: 10, CommentBody: "Comment 2"},
		{FilePath: "util.go", LineNumber: 3, CommentBody: "Comment 3"},
		{FilePath: "SUMMARY", LineNumber: 0, CommentBody: "Summary comment"},
	}

	grouped := groupCommentsByFile(comments)

	assert.Len(t, grouped, 3, "Should have 3 file groups")
	assert.Len(t, grouped["main.go"], 2, "main.go should have 2 comments")
	assert.Len(t, grouped["util.go"], 1, "util.go should have 1 comment")
	assert.Len(t, grouped["SUMMARY"], 1, "SUMMARY should have 1 comment")
}

func TestPrintDiffWithComments_Integration(t *testing.T) {
	// This is an integration test that verifies the function runs without panicking
	// Actual output is visual and would need manual verification

	diff := `diff --git a/main.go b/main.go
index 1234567..abcdefg 100644
--- a/main.go
+++ b/main.go
@@ -1,5 +1,6 @@
 package main

+import "fmt"
+
 func main() {
-	println("hello")
+	fmt.Println("hello")
 }`

	comments := []types.LineComment{
		{FilePath: "main.go", LineNumber: 3, CommentBody: "Good addition of fmt import", Importance: "LOW"},
		{FilePath: "main.go", LineNumber: 6, CommentBody: "Consider using log instead of fmt", Importance: "MEDIUM"},
		{FilePath: "SUMMARY", LineNumber: 0, CommentBody: "Overall changes look good"},
	}

	// This should not panic
	assert.NotPanics(t, func() {
		PrintDiffWithComments(diff, comments)
	})
}

func TestParseDiffForTerminal_LineNumbers(t *testing.T) {
	diff := `diff --git a/test.go b/test.go
index 1234567..abcdefg 100644
--- a/test.go
+++ b/test.go
@@ -10,5 +10,6 @@
 func test() {
+	// new line
 	return
 }`

	files := parseDiffForTerminal(diff)

	assert.Len(t, files, 1)

	// Find the added line
	var addedLine *DiffLine
	for i := range files[0].Lines {
		if files[0].Lines[i].Type == "added" {
			addedLine = &files[0].Lines[i]
			break
		}
	}

	assert.NotNil(t, addedLine, "Should find an added line")
	assert.Equal(t, 11, addedLine.NewLineNumber, "Added line should have correct line number")
	assert.Contains(t, addedLine.Content, "new line", "Added line should contain expected content")
}

func TestParseDiffForTerminal_DeletedLines(t *testing.T) {
	diff := `diff --git a/test.go b/test.go
index 1234567..abcdefg 100644
--- a/test.go
+++ b/test.go
@@ -1,5 +1,4 @@
 package test
-// old comment
 func main() {
 }`

	files := parseDiffForTerminal(diff)

	assert.Len(t, files, 1)

	// Find the deleted line
	var deletedLine *DiffLine
	for i := range files[0].Lines {
		if files[0].Lines[i].Type == "deleted" {
			deletedLine = &files[0].Lines[i]
			break
		}
	}

	assert.NotNil(t, deletedLine, "Should find a deleted line")
	assert.Contains(t, deletedLine.Content, "old comment", "Deleted line should contain expected content")
}

func TestParseDiffForTerminal_EmptyDiff(t *testing.T) {
	diff := ""
	files := parseDiffForTerminal(diff)
	assert.Len(t, files, 0, "Empty diff should return no files")
}

func TestParseDiffForTerminal_HunkHeaders(t *testing.T) {
	diff := `diff --git a/test.go b/test.go
index 1234567..abcdefg 100644
--- a/test.go
+++ b/test.go
@@ -1,3 +1,4 @@
 package test
+// comment
 func main() {
 }`

	files := parseDiffForTerminal(diff)

	assert.Len(t, files, 1)

	// Check for hunk header
	var hasHunk bool
	for _, line := range files[0].Lines {
		if line.Type == "hunk" && strings.HasPrefix(line.Content, "@@") {
			hasHunk = true
			break
		}
	}

	assert.True(t, hasHunk, "Should parse hunk headers")
}
