#!/usr/bin/env python3
"""
PR Triage Script

Analyzes AI-generated reviews for PRs requesting your review and categorizes them:
- NEEDS CLOSE INSPECTION: Has CRITICAL issues or multiple MEDIUM issues
- CURSORY LOOK: Only LOW importance comments or minor suggestions

Usage:
    python scripts/triage-prs.py [--server URL]
"""

import argparse
import json
import re
import sys
from dataclasses import dataclass
from typing import Optional
from urllib.request import urlopen, Request
from urllib.error import URLError


@dataclass
class PRSummary:
    owner: str
    repo: str
    number: int
    title: str
    author: str
    github_url: str
    review_url: str
    critical_count: int = 0
    medium_count: int = 0
    low_count: int = 0
    total_comments: int = 0
    error: Optional[str] = None

    @property
    def needs_close_inspection(self) -> bool:
        """PR needs close inspection if it has CRITICAL issues or 2+ MEDIUM issues."""
        return self.critical_count > 0 or self.medium_count >= 2

    @property
    def category(self) -> str:
        if self.error:
            return "ERROR"
        if self.critical_count > 0:
            return "🔴 CRITICAL"
        if self.medium_count >= 2:
            return "🟠 MULTIPLE MEDIUM"
        if self.medium_count == 1:
            return "🟡 MINOR ISSUES"
        return "🟢 LOOKS GOOD"


def fetch_json(url: str) -> dict:
    """Fetch JSON from a URL."""
    req = Request(url, headers={"Accept": "application/json"})
    with urlopen(req, timeout=30) as response:
        return json.loads(response.read().decode("utf-8"))


def fetch_html(url: str) -> str:
    """Fetch HTML content from a URL."""
    req = Request(url, headers={"Accept": "text/html"})
    with urlopen(req, timeout=30) as response:
        return response.read().decode("utf-8")


def extract_importance_counts(html: str) -> tuple[int, int, int, int]:
    """
    Extract importance counts from the review HTML.

    The review HTML contains a JavaScript section with the original comments
    serialized, including their importance levels (CRITICAL, MEDIUM, LOW).

    Returns: (critical, medium, low, total)
    """
    critical = 0
    medium = 0
    low = 0

    # Method 1: Look for importance markers in the JavaScript comments data
    # The format in the HTML is like: "CRITICAL}" or "MEDIUM}" or "LOW}"
    # These appear in the serialized comment data in the chat sidebar JS

    # Find the section with review comments (in the sendMessage function context)
    js_section_match = re.search(
        r'Here are the initial review comments you provided.*?---\s*(.*?)\s*---',
        html,
        re.DOTALL
    )

    if js_section_match:
        comments_data = js_section_match.group(1)
        # Count importance levels - they appear at the end of each comment like "... LOW}" or "CRITICAL}"
        critical = len(re.findall(r'\bCRITICAL\b', comments_data, re.IGNORECASE))
        medium = len(re.findall(r'\bMEDIUM\b', comments_data, re.IGNORECASE))
        low = len(re.findall(r'\bLOW\b', comments_data, re.IGNORECASE))

    # Method 2: Also scan the visible HTML for severity indicators
    # Some reviews might have explicit severity markers in comment text
    if critical == 0 and medium == 0 and low == 0:
        # Look for severity patterns in comment bodies
        comment_bodies = re.findall(r'<div class="comment-body">(.*?)</div>', html, re.DOTALL)
        for body in comment_bodies:
            body_upper = body.upper()
            if 'CRITICAL' in body_upper or 'SECURITY' in body_upper or 'VULNERABILITY' in body_upper:
                critical += 1
            elif 'BUG' in body_upper or 'ERROR' in body_upper or 'ISSUE' in body_upper:
                medium += 1

    # Count total comments from the comment counter
    total_match = re.search(r'Comment \d+ of (\d+)', html)
    total = int(total_match.group(1)) if total_match else (critical + medium + low)

    return critical, medium, low, total


def analyze_review_content(html: str) -> dict:
    """
    Analyze the review HTML content for severity indicators.

    Returns a dict with analysis results.
    """
    critical, medium, low, total = extract_importance_counts(html)

    # Additional heuristics from the review summary section
    summary_match = re.search(
        r'<div class="summary-body">(.*?)</div>',
        html,
        re.DOTALL
    )

    has_security_concerns = False
    has_suggestions = False

    if summary_match:
        summary = summary_match.group(1).lower()
        has_security_concerns = any(word in summary for word in [
            'security', 'vulnerability', 'injection', 'xss', 'csrf', 'auth'
        ])
        has_suggestions = 'suggestion' in html.lower() or '```suggestion' in html

    # Boost critical if security concerns found
    if has_security_concerns and critical == 0:
        critical = 1

    return {
        'critical': critical,
        'medium': medium,
        'low': low,
        'total': total,
        'has_suggestions': has_suggestions,
        'has_security_concerns': has_security_concerns,
    }


def triage_prs(server_url: str) -> list[PRSummary]:
    """
    Fetch PRs and analyze their reviews.

    Returns a list of PRSummary objects.
    """
    # Fetch all PRs
    prs_url = f"{server_url}/api/prs"
    print(f"Fetching PRs from {prs_url}...")

    try:
        prs = fetch_json(prs_url)
    except URLError as e:
        print(f"Error fetching PRs: {e}", file=sys.stderr)
        print(f"Make sure the server is running at {server_url}", file=sys.stderr)
        sys.exit(1)

    # API returns a list directly
    if not isinstance(prs, list):
        prs = prs.get("prs", [])

    # Filter for PRs that need my review (is_mine=false) with completed reviews
    review_prs = [
        pr for pr in prs
        if not pr.get("is_mine", True)
        and pr.get("status") == "completed"
        and pr.get("review_url")
    ]

    print(f"Found {len(review_prs)} PRs with completed reviews that need your review\n")

    summaries = []

    for pr in review_prs:
        summary = PRSummary(
            owner=pr.get("owner", ""),
            repo=pr.get("repo", ""),
            number=pr.get("number", 0),
            title=pr.get("title", "Unknown"),
            author=pr.get("author", "Unknown"),
            github_url=pr.get("github_url", ""),
            review_url=pr.get("review_url", ""),
        )

        # Fetch and analyze the review HTML
        review_url = f"{server_url}{summary.review_url}"
        try:
            print(f"  Analyzing {summary.owner}/{summary.repo}#{summary.number}...")
            html = fetch_html(review_url)
            analysis = analyze_review_content(html)

            summary.critical_count = analysis['critical']
            summary.medium_count = analysis['medium']
            summary.low_count = analysis['low']
            summary.total_comments = analysis['total']

        except Exception as e:
            summary.error = str(e)

        summaries.append(summary)

    return summaries


def print_results(summaries: list[PRSummary]):
    """Print the triage results in a readable format."""
    if not summaries:
        print("No PRs found that need your review with completed AI reviews.")
        return

    # Sort: critical first, then by medium count, then by total comments
    summaries.sort(key=lambda s: (-s.critical_count, -s.medium_count, -s.total_comments))

    # Separate into categories
    needs_inspection = [s for s in summaries if s.needs_close_inspection]
    quick_review = [s for s in summaries if not s.needs_close_inspection and not s.error]
    errors = [s for s in summaries if s.error]

    print("=" * 70)
    print("PR TRIAGE RESULTS")
    print("=" * 70)

    if needs_inspection:
        print("\n🔴 NEEDS CLOSE INSPECTION")
        print("-" * 70)
        for s in needs_inspection:
            print(f"\n  {s.category}: {s.owner}/{s.repo}#{s.number}")
            print(f"  Title:  {s.title[:60]}{'...' if len(s.title) > 60 else ''}")
            print(f"  Author: {s.author}")
            print(f"  Issues: {s.critical_count} critical, {s.medium_count} medium, {s.low_count} low")
            print(f"  GitHub: {s.github_url}")

    if quick_review:
        print("\n🟢 PROBABLY QUICK APPROVE (cursory look recommended)")
        print("-" * 70)
        for s in quick_review:
            print(f"\n  {s.category}: {s.owner}/{s.repo}#{s.number}")
            print(f"  Title:  {s.title[:60]}{'...' if len(s.title) > 60 else ''}")
            print(f"  Author: {s.author}")
            if s.total_comments > 0:
                print(f"  Comments: {s.total_comments} total ({s.medium_count} medium, {s.low_count} low)")
            else:
                print(f"  Comments: No significant issues found")
            print(f"  GitHub: {s.github_url}")

    if errors:
        print("\n⚠️  ERRORS (couldn't analyze)")
        print("-" * 70)
        for s in errors:
            print(f"\n  {s.owner}/{s.repo}#{s.number}: {s.error}")

    # Summary
    print("\n" + "=" * 70)
    print("SUMMARY")
    print("=" * 70)
    print(f"  Total PRs awaiting your review: {len(summaries)}")
    print(f"  Need close inspection:          {len(needs_inspection)}")
    print(f"  Probably quick approve:         {len(quick_review)}")
    if errors:
        print(f"  Couldn't analyze:               {len(errors)}")

    # Recommendation
    print("\n💡 RECOMMENDATION:")
    if needs_inspection:
        top = needs_inspection[0]
        print(f"   Start with {top.owner}/{top.repo}#{top.number} - it has the most critical issues")
    elif quick_review:
        print(f"   All {len(quick_review)} PRs look straightforward - quick reviews should suffice")


def main():
    parser = argparse.ArgumentParser(
        description="Triage PRs based on AI review severity"
    )
    parser.add_argument(
        "--server", "-s",
        default="http://localhost:7769",
        help="PR Review Server URL (default: http://localhost:7769)"
    )
    parser.add_argument(
        "--json", "-j",
        action="store_true",
        help="Output results as JSON"
    )

    args = parser.parse_args()

    summaries = triage_prs(args.server)

    if args.json:
        output = [
            {
                "owner": s.owner,
                "repo": s.repo,
                "number": s.number,
                "title": s.title,
                "author": s.author,
                "github_url": s.github_url,
                "category": s.category,
                "needs_close_inspection": s.needs_close_inspection,
                "critical_count": s.critical_count,
                "medium_count": s.medium_count,
                "low_count": s.low_count,
                "total_comments": s.total_comments,
                "error": s.error,
            }
            for s in summaries
        ]
        print(json.dumps(output, indent=2))
    else:
        print_results(summaries)


if __name__ == "__main__":
    main()
