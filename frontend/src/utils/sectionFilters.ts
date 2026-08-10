import type { PR } from '@/types/pr';
import type { SectionFilters } from '@/types/sections';
import { getTeamFilterName } from '@/utils/teamFilters';

/**
 * A subset of section filters. The filter bar at the top of the page and each
 * configured section both express themselves this way, so the same matcher
 * applies to both and their constraints simply intersect.
 */
export type PRFilterCriteria = Partial<SectionFilters>;

export function prMatchesSearch(pr: PR, search: string): boolean {
  const term = search.trim().toLowerCase();
  if (!term) return true;
  return (
    pr.title.toLowerCase().includes(term) ||
    pr.repo.toLowerCase().includes(term) ||
    pr.owner.toLowerCase().includes(term) ||
    pr.author.toLowerCase().includes(term) ||
    pr.number.toString().includes(term)
  );
}

/**
 * Whether a PR satisfies every populated facet of `criteria`. Values within a
 * facet are OR'd; facets are AND'd; empty facets impose no constraint.
 */
export function prMatchesFilters(
  pr: PR,
  criteria: PRFilterCriteria,
  username?: string
): boolean {
  const { authorship = 'any', teams, repos, authors, states, search } = criteria;

  if (authorship === 'mine' && !pr.is_mine) return false;
  if (authorship === 'others' && pr.is_mine) return false;

  if (search && !prMatchesSearch(pr, search)) return false;

  if (teams && teams.length > 0) {
    if (!pr.via_teams.some((t) => teams.includes(getTeamFilterName(t, username)))) return false;
  }

  if (repos && repos.length > 0) {
    if (!repos.includes(`${pr.owner}/${pr.repo}`)) return false;
  }

  if (authors && authors.length > 0) {
    if (!authors.includes(pr.author)) return false;
  }

  if (states && states.length > 0) {
    if (!states.includes(pr.draft ? 'draft' : 'ready')) return false;
  }

  return true;
}

/** Newest first, with PRs of unknown creation date sorted last. */
export function sortPRsByNewest(prs: PR[]): PR[] {
  return [...prs].sort((a, b) => {
    if (!a.created_at && !b.created_at) return 0;
    if (!a.created_at) return 1;
    if (!b.created_at) return -1;
    return new Date(b.created_at).getTime() - new Date(a.created_at).getTime();
  });
}

export function filterAndSortPRs(
  prs: PR[],
  criteria: PRFilterCriteria,
  username?: string
): PR[] {
  return sortPRsByNewest(prs.filter((pr) => prMatchesFilters(pr, criteria, username)));
}

export function prKey(pr: PR): string {
  return `${pr.owner}/${pr.repo}/${pr.number}`;
}
