package github

import (
	"testing"
)

const samplePRBody = `<!--
NOTE: DO NOT DELETE any of the headers if the section is required. Doing so may result in a CI failure.
-->

## JIRA Issue

<!--
JIRA Issue Header
Required.

You can use shorthand to auto-generate link (example 'PROJ-1234') instead of filling out manually
-->

[Feature request handling](https://your-org.atlassian.net/browse/PROJ-2036)

## Related PR(s)

<!--
Related PR(s) Header

Any related PRs. Link them here.

Delete this section if there are no related PRs.
-->

## Summary / Background
Implement feature handling for the mobile application
<!--
Summary / Background Header
Required if your PR fails the "File Count Check". Use this as justification.

Use this section to give a high level overview of the "why" and "what" for your changes. This section can be
safely deleted if there is no additional information you need to provide.
-->

## Code Changes

<!--
Code Changes Header
Required.

* Highlight significant design decisions
* Highlight edge cases
* Highlight new models, migrations, celery tasks, etc.
-->

- one
- two
- three

## Related Flags / Switches

<!--
Related Flags / Switches Header
Required if your changes are behind a flag or switch (Feature Flag / Split Test).

This section should include a list of flags or switches that affect code paths for your changes.
-->

## Affected Areas (Testing Scope)

<!--
Affected Areas Header
Required if you have code changes (non-test Python/Typescript/HTML/Javascript/TSX/etc) files.

This section should comprise a list of scenarios you need to keep an eye on when testing this particular PR.
-->

## DevOps Considerations (Security / Performance / Infrastructure)

<!--
DevOps Considerations Header
Optional.

This section is for anything you would like to draw attention to for DevOps.
-->`

func TestParsePullRequestDescription(t *testing.T) {
	description := ParsePullRequestDescription(samplePRBody)

	expectedJira := "[Feature request handling](https://your-org.atlassian.net/browse/PROJ-2036)"
	if description.JiraIssue != expectedJira {
		t.Errorf("Expected Jira issue '%s', but got '%s'", expectedJira, description.JiraIssue)
	}

	expectedSummary := "Implement feature handling for the mobile application"
	if description.Summary != expectedSummary {
		t.Errorf("Expected Summary '%s', but got '%s'", expectedSummary, description.Summary)
	}

	expectedCodeChanges := "- one\n- two\n- three"
	if description.CodeChanges != expectedCodeChanges {
		t.Errorf("Expected Code Changes '%s', but got '%s'", expectedCodeChanges, description.CodeChanges)
	}

	if description.RelatedPRs != "" {
		t.Errorf("Expected Related PRs to be empty, but got '%s'", description.RelatedPRs)
	}
}
