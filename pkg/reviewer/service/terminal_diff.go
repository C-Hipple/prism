package service

import (
	"fmt"
	"strings"

	"github.com/fatih/color"
	"pr-review-server/pkg/reviewer/types"
)

var (
	diffHeader    = color.New(color.FgCyan, color.Bold)
	addedLine     = color.New(color.FgGreen)
	deletedLine   = color.New(color.FgRed)
	contextLine   = color.New(color.FgWhite)
	commentHeader = color.New(color.FgYellow, color.Bold)
	commentBody   = color.New(color.FgYellow)
	hunkHeader    = color.New(color.FgCyan)
)

// DiffLine represents a single line in a diff
type DiffLine struct {
	Type          string // "added", "deleted", "context", "hunk"
	Content       string
	OldLineNumber int
	NewLineNumber int
}

// DiffFileStructure represents a file in the diff with its lines
type DiffFileStructure struct {
	Path  string
	Lines []DiffLine
}

// PrintDiffWithComments prints the diff to the terminal with comments embedded at relevant lines
func PrintDiffWithComments(diff string, comments []types.LineComment) {
	files := parseDiffForTerminal(diff)
	commentsByFile := groupCommentsByFile(comments)

	for _, file := range files {
		// Print file header
		diffHeader.Printf("\n━━━ %s ━━━\n", file.Path)

		fileComments := commentsByFile[file.Path]
		commentsByLine := make(map[int][]types.LineComment)
		for _, c := range fileComments {
			commentsByLine[c.LineNumber] = append(commentsByLine[c.LineNumber], c)
		}

		// Print summary/general comments for this file first
		for _, comment := range fileComments {
			if comment.FilePath == "SUMMARY" || comment.FilePath == "GENERAL" || comment.LineNumber == 0 {
				printComment(comment)
			}
		}

		// Print diff lines with embedded comments
		for _, line := range file.Lines {
			printDiffLine(line)

			// Print comments for this line
			if line.NewLineNumber > 0 {
				if comments, ok := commentsByLine[line.NewLineNumber]; ok {
					for _, comment := range comments {
						if comment.LineNumber > 0 { // Skip summary/general comments (already printed)
							printComment(comment)
						}
					}
				}
			}
		}
	}

	// Print any summary or general comments that weren't file-specific
	var globalComments []types.LineComment
	for _, comment := range comments {
		if comment.FilePath == "SUMMARY" || comment.FilePath == "GENERAL" {
			globalComments = append(globalComments, comment)
		}
	}

	if len(globalComments) > 0 {
		diffHeader.Printf("\n━━━ Review Summary ━━━\n")
		for _, comment := range globalComments {
			printComment(comment)
		}
	}
}

// parseDiffForTerminal parses a raw diff string into structured format for terminal display
func parseDiffForTerminal(diff string) []DiffFileStructure {
	var files []DiffFileStructure
	lines := strings.Split(diff, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	var currentFile *DiffFileStructure
	var oldLine, newLine int

	for _, line := range lines {
		if strings.HasPrefix(line, "diff --git") {
			if currentFile != nil {
				files = append(files, *currentFile)
			}
			parts := strings.Split(line, " ")
			if len(parts) >= 3 {
				path := strings.TrimPrefix(parts[2], "a/")
				currentFile = &DiffFileStructure{Path: path}
			}
			continue
		}
		if strings.HasPrefix(line, "---") || strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "index") {
			continue
		}
		if currentFile == nil {
			continue
		}

		if strings.HasPrefix(line, "@@") {
			// Parse hunk header like "@@ -1,5 +1,6 @@"
			parts := strings.Split(line, " ")
			if len(parts) >= 3 {
				oldRange := strings.Split(strings.TrimPrefix(parts[1], "-"), ",")
				newRange := strings.Split(strings.TrimPrefix(parts[2], "+"), ",")
				if len(oldRange) > 0 {
					_, _ = fmt.Sscanf(oldRange[0], "%d", &oldLine)
				}
				if len(newRange) > 0 {
					_, _ = fmt.Sscanf(newRange[0], "%d", &newLine)
				}
			}
			currentFile.Lines = append(currentFile.Lines, DiffLine{
				Type:    "hunk",
				Content: line,
			})
			continue
		}

		switch {
		case strings.HasPrefix(line, "+"):
			currentFile.Lines = append(currentFile.Lines, DiffLine{
				Type:          "added",
				Content:       strings.TrimPrefix(line, "+"),
				NewLineNumber: newLine,
			})
			newLine++
		case strings.HasPrefix(line, "-"):
			currentFile.Lines = append(currentFile.Lines, DiffLine{
				Type:          "deleted",
				Content:       strings.TrimPrefix(line, "-"),
				OldLineNumber: oldLine,
			})
			oldLine++
		default:
			content := line
			if strings.HasPrefix(line, " ") {
				content = strings.TrimPrefix(line, " ")
			}
			currentFile.Lines = append(currentFile.Lines, DiffLine{
				Type:          "context",
				Content:       content,
				OldLineNumber: oldLine,
				NewLineNumber: newLine,
			})
			oldLine++
			newLine++
		}
	}
	if currentFile != nil && len(currentFile.Lines) > 0 {
		files = append(files, *currentFile)
	}

	return files
}

// groupCommentsByFile groups comments by their file path
func groupCommentsByFile(comments []types.LineComment) map[string][]types.LineComment {
	result := make(map[string][]types.LineComment)
	for _, comment := range comments {
		result[comment.FilePath] = append(result[comment.FilePath], comment)
	}
	return result
}

// printDiffLine prints a single diff line with appropriate coloring
func printDiffLine(line DiffLine) {
	switch line.Type {
	case "added":
		addedLine.Printf("+ %s\n", line.Content)
	case "deleted":
		deletedLine.Printf("- %s\n", line.Content)
	case "hunk":
		hunkHeader.Printf("%s\n", line.Content)
	case "context":
		contextLine.Printf("  %s\n", line.Content)
	}
}

// printComment prints a comment with formatting
func printComment(comment types.LineComment) {
	commentHeader.Printf("  💬 Review Comment")
	if comment.LineNumber > 0 {
		commentHeader.Printf(" (line %d)", comment.LineNumber)
	}
	if comment.Importance != "" {
		importance := strings.ToUpper(comment.Importance)
		switch importance {
		case "CRITICAL":
			color.New(color.FgRed, color.Bold).Printf(" [%s]", importance)
		case "MEDIUM":
			color.New(color.FgYellow, color.Bold).Printf(" [%s]", importance)
		case "LOW":
			color.New(color.FgGreen, color.Bold).Printf(" [%s]", importance)
		}
	}
	fmt.Println()

	// Print comment body with indentation
	commentLines := strings.Split(comment.CommentBody, "\n")
	for _, line := range commentLines {
		commentBody.Printf("     %s\n", line)
	}
	fmt.Println()
}
