import { describe, expect, it } from 'vitest';
import type { PR } from '@/types/pr';
import { filterAndSortPRs, prMatchesFilters, sortPRsByNewest } from './sectionFilters';

const makePR = (partial: Partial<PR>): PR => ({
  owner: 'acme',
  repo: 'platform',
  number: 1,
  commit_sha: 'abc123',
  last_reviewed_at: null,
  review_html_path: '',
  github_url: '',
  review_url: '',
  status: 'pending',
  title: 'Example PR',
  author: 'alice',
  generating_since: null,
  approval_count: 0,
  my_review_status: '',
  draft: false,
  ci_state: 'unknown',
  ci_failed_checks: [],
  created_at: '2026-04-15T12:00:00Z',
  is_mine: false,
  via_teams: [],
  critical_count: 0,
  medium_count: 0,
  low_count: 0,
  notes: '',
  ...partial,
});

describe('prMatchesFilters', () => {
  it('treats empty criteria as no constraint', () => {
    expect(prMatchesFilters(makePR({}), {})).toBe(true);
    expect(
      prMatchesFilters(makePR({}), { teams: [], repos: [], authors: [], states: [], search: '' })
    ).toBe(true);
  });

  it('filters on authorship', () => {
    const mine = makePR({ is_mine: true });
    const theirs = makePR({ is_mine: false });

    expect(prMatchesFilters(mine, { authorship: 'mine' })).toBe(true);
    expect(prMatchesFilters(theirs, { authorship: 'mine' })).toBe(false);
    expect(prMatchesFilters(mine, { authorship: 'others' })).toBe(false);
    expect(prMatchesFilters(theirs, { authorship: 'others' })).toBe(true);
    expect(prMatchesFilters(mine, { authorship: 'any' })).toBe(true);
  });

  it('ORs within a facet and ANDs across facets', () => {
    const pr = makePR({ repo: 'platform', author: 'bob', via_teams: ['Platform:pending'] });

    expect(prMatchesFilters(pr, { repos: ['acme/api', 'acme/platform'] })).toBe(true);
    expect(prMatchesFilters(pr, { repos: ['acme/api'] })).toBe(false);
    expect(prMatchesFilters(pr, { authors: ['bob', 'carol'] })).toBe(true);
    expect(prMatchesFilters(pr, { authors: ['carol'] })).toBe(false);
    expect(prMatchesFilters(pr, { teams: ['Platform'] })).toBe(true);
    expect(prMatchesFilters(pr, { teams: ['Growth'] })).toBe(false);

    // Every populated facet has to match.
    expect(prMatchesFilters(pr, { repos: ['acme/platform'], authors: ['bob'] })).toBe(true);
    expect(prMatchesFilters(pr, { repos: ['acme/platform'], authors: ['carol'] })).toBe(false);
  });

  it('resolves the personal team sentinel against the current user', () => {
    const pr = makePR({ via_teams: ['__PERSONAL__'] });

    expect(prMatchesFilters(pr, { teams: ['@alice'] }, 'alice')).toBe(true);
    expect(prMatchesFilters(pr, { teams: ['@alice'] }, 'bob')).toBe(false);
  });

  it('filters on draft vs ready state', () => {
    expect(prMatchesFilters(makePR({ draft: true }), { states: ['draft'] })).toBe(true);
    expect(prMatchesFilters(makePR({ draft: true }), { states: ['ready'] })).toBe(false);
    expect(prMatchesFilters(makePR({ draft: false }), { states: ['ready'] })).toBe(true);
    expect(prMatchesFilters(makePR({ draft: false }), { states: ['draft', 'ready'] })).toBe(true);
  });

  it('searches title, repo, owner, author and number case-insensitively', () => {
    const pr = makePR({ number: 4321, title: 'Fix Billing', repo: 'platform', author: 'bob' });

    for (const term of ['billing', 'PLATFORM', 'acme', 'Bob', '432']) {
      expect(prMatchesFilters(pr, { search: term })).toBe(true);
    }
    expect(prMatchesFilters(pr, { search: 'chat' })).toBe(false);
    expect(prMatchesFilters(pr, { search: '   ' })).toBe(true);
  });
});

describe('sortPRsByNewest', () => {
  it('sorts newest first with unknown dates last, leaving the input untouched', () => {
    const prs = [
      makePR({ number: 1, created_at: '2026-01-01T00:00:00Z' }),
      makePR({ number: 2, created_at: null }),
      makePR({ number: 3, created_at: '2026-06-01T00:00:00Z' }),
    ];

    expect(sortPRsByNewest(prs).map((pr) => pr.number)).toEqual([3, 1, 2]);
    expect(prs.map((pr) => pr.number)).toEqual([1, 2, 3]);
  });
});

describe('filterAndSortPRs', () => {
  it('applies the criteria and returns newest first', () => {
    const prs = [
      makePR({ number: 1, author: 'bob', created_at: '2026-01-01T00:00:00Z' }),
      makePR({ number: 2, author: 'carol', created_at: '2026-02-01T00:00:00Z' }),
      makePR({ number: 3, author: 'bob', created_at: '2026-03-01T00:00:00Z' }),
    ];

    expect(filterAndSortPRs(prs, { authors: ['bob'] }).map((pr) => pr.number)).toEqual([3, 1]);
  });
});
