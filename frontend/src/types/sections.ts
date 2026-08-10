import type { PRStateFilter } from '@/hooks/useUrlFilters';

/** Which PRs a section accepts based on who authored them. */
export type SectionAuthorship = 'any' | 'mine' | 'others';

/**
 * Filters for a user-defined section. Values within a facet are OR'd, facets
 * are AND'd, and an empty facet means "no constraint".
 */
export interface SectionFilters {
  authorship: SectionAuthorship;
  /** Team display names, as produced by getTeamFilterName. */
  teams: string[];
  /** Repos in "owner/repo" form. */
  repos: string[];
  /** GitHub logins of PR authors. */
  authors: string[];
  states: PRStateFilter[];
  /** Free-text match over title/repo/owner/author/number. */
  search: string;
}

export interface SectionConfig {
  id: string;
  title: string;
  /** Persisted per section so a collapsed section stays collapsed on reload. */
  collapsed: boolean;
  filters: SectionFilters;
}

export interface SectionsConfig {
  version: number;
  sections: SectionConfig[];
  /**
   * Append a catch-all section listing PRs that match no configured section,
   * so narrow filters never make a PR silently disappear from the page.
   */
  showUncategorized: boolean;
}
