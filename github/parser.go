package github

import (
	"regexp"
	"strings"
)

// PullRequestDescription holds the parsed content of a PR description.
type PullRequestDescription struct {
	JiraIssue              string
	RelatedPRs             string
	Summary                string
	CodeChanges            string
	RelatedFlagsOrSwitches string
	AffectedAreas          string
	DevOpsConsiderations   string
}

// ParsePullRequestDescription extracts relevant sections from a PR description.
func ParsePullRequestDescription(body string) *PullRequestDescription {
	// First, remove all HTML comments
	re := regexp.MustCompile(`<!--(.|\n)*?-->`)
	body = re.ReplaceAllString(body, "")

	description := &PullRequestDescription{}

	sections := map[string]*string{
		"JIRA Issue":                     &description.JiraIssue,
		"Related PR(s)":                  &description.RelatedPRs,
		"Summary / Background":           &description.Summary,
		"Code Changes":                   &description.CodeChanges,
		"Related Flags / Switches":       &description.RelatedFlagsOrSwitches,
		"Affected Areas (Testing Scope)": &description.AffectedAreas,
		"DevOps Considerations (Security / Performance / Infrastructure)": &description.DevOpsConsiderations,
	}

	// Normalize line endings
	body = strings.ReplaceAll(body, "\r\n", "\n")
	lines := strings.Split(body, "\n")

	var currentSection *string
	var content strings.Builder

	// Pre-compile regex for header detection
	headerRe := regexp.MustCompile(`^##\s`)

	for _, line := range lines {
		isHeader := false
		for header, field := range sections {
			// Headers are expected to be ## Some Header
			if strings.HasPrefix(line, "## "+header) {
				if currentSection != nil {
					*currentSection = strings.TrimSpace(content.String())
				}
				content.Reset()
				currentSection = field
				isHeader = true
				break
			}
		}

		if !isHeader {
			// Also ignore the comment blocks
			if !strings.HasPrefix(strings.TrimSpace(line), "<!--") && !strings.HasPrefix(strings.TrimSpace(line), "-->") {
				// We don't want to include the header in the content
				if !headerRe.MatchString(line) {
					content.WriteString(line + "\n")
				}
			}
		}
	}

	if currentSection != nil {
		*currentSection = strings.TrimSpace(content.String())
	}

	return description
}
