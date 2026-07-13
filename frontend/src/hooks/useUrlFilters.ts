import { useCallback, useEffect, useRef, useState } from 'react';

export interface UrlFilters {
  searchTerm: string;
  selectedTeams: string[];
  selectedRepos: string[];
}

// How long to wait after the last keystroke before pushing the search term
// into the URL. Discrete filter changes (teams/repos) push immediately.
const SEARCH_PUSH_DEBOUNCE_MS = 400;

function readFiltersFromUrl(): UrlFilters {
  const params = new URLSearchParams(window.location.search);
  return {
    searchTerm: params.get('q') ?? '',
    selectedTeams: params.getAll('team'),
    selectedRepos: params.getAll('repo'),
  };
}

function buildUrl(filters: UrlFilters): string {
  const params = new URLSearchParams(window.location.search);
  params.delete('q');
  params.delete('team');
  params.delete('repo');
  if (filters.searchTerm) params.set('q', filters.searchTerm);
  for (const team of filters.selectedTeams) params.append('team', team);
  for (const repo of filters.selectedRepos) params.append('repo', repo);
  const search = params.toString();
  return `${window.location.pathname}${search ? `?${search}` : ''}${window.location.hash}`;
}

/**
 * Filter state (search term, team filters, repo filters) mirrored into URL
 * query params (?q=...&team=...&repo=...) as history entries, so filter
 * state survives reloads, is shareable, and back/forward navigates it.
 */
export function useUrlFilters() {
  const [filters, setFilters] = useState<UrlFilters>(readFiltersFromUrl);
  const filtersRef = useRef(filters);
  const searchDebounceRef = useRef<number | undefined>(undefined);

  useEffect(() => {
    const onPopState = () => {
      // A pending search push would clobber the state we're navigating to.
      window.clearTimeout(searchDebounceRef.current);
      const restored = readFiltersFromUrl();
      filtersRef.current = restored;
      setFilters(restored);
    };
    window.addEventListener('popstate', onPopState);
    return () => {
      window.removeEventListener('popstate', onPopState);
      window.clearTimeout(searchDebounceRef.current);
    };
  }, []);

  const pushUrl = useCallback(() => {
    const url = buildUrl(filtersRef.current);
    if (url !== `${window.location.pathname}${window.location.search}${window.location.hash}`) {
      window.history.pushState(null, '', url);
    }
  }, []);

  const applyFilters = useCallback(
    (next: UrlFilters, { debounce = false } = {}) => {
      filtersRef.current = next;
      setFilters(next);
      window.clearTimeout(searchDebounceRef.current);
      if (debounce) {
        searchDebounceRef.current = window.setTimeout(pushUrl, SEARCH_PUSH_DEBOUNCE_MS);
      } else {
        pushUrl();
      }
    },
    [pushUrl]
  );

  const setSearchTerm = useCallback(
    (searchTerm: string) => applyFilters({ ...filtersRef.current, searchTerm }, { debounce: true }),
    [applyFilters]
  );

  const setSelectedTeams = useCallback(
    (selectedTeams: string[]) => applyFilters({ ...filtersRef.current, selectedTeams }),
    [applyFilters]
  );

  const setSelectedRepos = useCallback(
    (selectedRepos: string[]) => applyFilters({ ...filtersRef.current, selectedRepos }),
    [applyFilters]
  );

  return {
    searchTerm: filters.searchTerm,
    selectedTeams: filters.selectedTeams,
    selectedRepos: filters.selectedRepos,
    setSearchTerm,
    setSelectedTeams,
    setSelectedRepos,
  };
}
