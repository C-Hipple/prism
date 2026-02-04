#!/usr/bin/env python3
"""
Backfill importance counts for existing reviews.

This script:
1. Fetches all PRs from the API
2. For each completed PR with a review, fetches the HTML
3. Parses importance counts from the HTML
4. Updates the database directly via SQLite

Usage:
    python scripts/backfill-importance.py [--db PATH]
"""

import argparse
import re
import sqlite3
import sys
from urllib.request import urlopen, Request


def fetch_json(url: str) -> list:
    """Fetch JSON from a URL."""
    import json
    req = Request(url, headers={"Accept": "application/json"})
    with urlopen(req, timeout=30) as response:
        return json.loads(response.read().decode("utf-8"))


def fetch_html(url: str) -> str:
    """Fetch HTML content from a URL."""
    req = Request(url, headers={"Accept": "text/html"})
    with urlopen(req, timeout=30) as response:
        return response.read().decode("utf-8")


def extract_importance_counts(html: str) -> tuple[int, int, int]:
    """
    Extract importance counts from the review HTML.
    Returns: (critical, medium, low)
    """
    critical = 0
    medium = 0
    low = 0

    # Look for importance markers in the JavaScript comments data
    js_section_match = re.search(
        r'Here are the initial review comments you provided.*?---\s*(.*?)\s*---',
        html,
        re.DOTALL
    )

    if js_section_match:
        comments_data = js_section_match.group(1)
        critical = len(re.findall(r'\bCRITICAL\b', comments_data, re.IGNORECASE))
        medium = len(re.findall(r'\bMEDIUM\b', comments_data, re.IGNORECASE))
        low = len(re.findall(r'\bLOW\b', comments_data, re.IGNORECASE))

    return critical, medium, low


def backfill(server_url: str, db_path: str):
    """Backfill importance counts for all completed PRs."""
    print(f"Fetching PRs from {server_url}/api/prs...")
    prs = fetch_json(f"{server_url}/api/prs")

    # Filter for completed PRs with reviews
    completed_prs = [
        pr for pr in prs
        if pr.get("status") == "completed" and pr.get("review_url")
    ]

    print(f"Found {len(completed_prs)} completed PRs with reviews")

    # Connect to database
    conn = sqlite3.connect(db_path)
    cursor = conn.cursor()

    updated = 0
    errors = 0

    for pr in completed_prs:
        owner = pr.get("owner")
        repo = pr.get("repo")
        number = pr.get("number")
        review_url = pr.get("review_url")

        try:
            html = fetch_html(f"{server_url}{review_url}")
            critical, medium, low = extract_importance_counts(html)

            # Update database
            cursor.execute("""
                UPDATE prs
                SET critical_count = ?, medium_count = ?, low_count = ?
                WHERE repo_owner = ? AND repo_name = ? AND pr_number = ?
            """, (critical, medium, low, owner, repo, number))

            if cursor.rowcount > 0:
                print(f"  Updated {owner}/{repo}#{number}: critical={critical}, medium={medium}, low={low}")
                updated += 1
            else:
                print(f"  No rows updated for {owner}/{repo}#{number}")

        except Exception as e:
            print(f"  Error processing {owner}/{repo}#{number}: {e}")
            errors += 1

    conn.commit()
    conn.close()

    print(f"\nBackfill complete: {updated} updated, {errors} errors")


def main():
    parser = argparse.ArgumentParser(description="Backfill importance counts for existing reviews")
    parser.add_argument("--server", "-s", default="http://localhost:8080", help="Server URL")
    parser.add_argument("--db", "-d", default="./data/pr-review.db", help="Database path")
    args = parser.parse_args()

    backfill(args.server, args.db)


if __name__ == "__main__":
    main()
