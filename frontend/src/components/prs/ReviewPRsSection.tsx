import { useEffect, useState } from 'react';
import { usePRs } from '@/hooks/usePRs';
import { useCurrentUser } from '@/hooks/useCurrentUser';
import { useTelemetry } from '@/hooks/useTelemetry';
import { PRTable } from './PRTable';
import { LoadingSpinner, ErrorMessage } from '@/components/common';
import { PR } from '@/types/pr';
import { getTeamFilterName } from '@/utils/teamFilters';
import { TriageFilter, categorizePR } from './triageUtils';
import type { PRStateFilter } from '@/hooks/useUrlFilters';
import './SectionHeader.scss';

interface ReviewPRsSectionProps {
  searchTerm?: string;
  triageFilter?: TriageFilter | null;
  selectedTeams?: string[];
  selectedRepos?: string[];
  selectedStates?: PRStateFilter[];
}

interface FilterOptions {
  searchTerm: string;
  selectedTeams?: string[];
  selectedRepos?: string[];
  selectedStates?: PRStateFilter[];
  username?: string;
}

function filterAndSortPRs(prs: PR[], { searchTerm, selectedTeams, selectedRepos, selectedStates, username }: FilterOptions): PR[] {
  return prs
    .filter((pr) => {
      if (searchTerm) {
        const lowerTerm = searchTerm.toLowerCase();
        const matchesSearch =
          pr.title.toLowerCase().includes(lowerTerm) ||
          pr.repo.toLowerCase().includes(lowerTerm) ||
          pr.owner.toLowerCase().includes(lowerTerm) ||
          pr.author.toLowerCase().includes(lowerTerm) ||
          pr.number.toString().includes(lowerTerm);
        if (!matchesSearch) return false;
      }

      if (selectedTeams && selectedTeams.length > 0) {
        if (!pr.via_teams.some(t => selectedTeams.includes(getTeamFilterName(t, username)))) return false;
      }

      if (selectedRepos && selectedRepos.length > 0) {
        if (!selectedRepos.includes(`${pr.owner}/${pr.repo}`)) return false;
      }

      if (selectedStates && selectedStates.length > 0) {
        if (!selectedStates.includes(pr.draft ? 'draft' : 'ready')) return false;
      }

      return true;
    })
    .sort((a, b) => {
      if (!a.created_at && !b.created_at) return 0;
      if (!a.created_at) return 1;
      if (!b.created_at) return -1;
      return new Date(b.created_at).getTime() - new Date(a.created_at).getTime();
    });
}

export function ReviewPRsSection({
  searchTerm = '',
  triageFilter = null,
  selectedTeams = [],
  selectedRepos = [],
  selectedStates = [],
}: ReviewPRsSectionProps) {
  const { data: prs, isLoading, error } = usePRs();
  const { data: currentUser } = useCurrentUser();
  const { track } = useTelemetry();
  // Collapse state is deliberately client-side only (per issue #30); hidden
  // membership itself is server-persisted per user.
  const [hiddenExpanded, setHiddenExpanded] = useState(false);

  useEffect(() => {
    track('view_review_prs_page');
  }, [track]);

  const filterOpts: FilterOptions = {
    searchTerm,
    selectedTeams,
    selectedRepos,
    selectedStates,
    username: currentUser?.github_username,
  };

  // Each PR lands in exactly one section: hidden > via_manual > is_mine >
  // review. User-hidden rows leave the top three and collect in the Hidden
  // section at the bottom instead.
  const allPRs = prs || [];
  const visiblePRs = allPRs.filter(pr => !pr.hidden);
  const hasHiddenPRs = allPRs.length > visiblePRs.length;
  const hiddenPRs = filterAndSortPRs(allPRs.filter(pr => pr.hidden), filterOpts);
  const requestedPRs = filterAndSortPRs(visiblePRs.filter(pr => pr.via_manual), filterOpts);
  const hasRequestedPRs = visiblePRs.some(pr => pr.via_manual);
  const myPRs = filterAndSortPRs(visiblePRs.filter(pr => !pr.via_manual && pr.is_mine), filterOpts);

  // Apply triage filter to review PRs
  let reviewPRs = filterAndSortPRs(visiblePRs.filter(pr => !pr.via_manual && !pr.is_mine), filterOpts);
  if (triageFilter) {
    reviewPRs = reviewPRs.filter(pr => {
      // Only filter completed PRs by triage category
      if (pr.status !== 'completed') return true;
      return categorizePR(pr) === triageFilter;
    });
  }

  const showSyncBanner = !isLoading && !error && allPRs.length === 0;

  return (
    <>
      {showSyncBanner && (
        <div className="review-prs__sync-banner">
          <strong>Syncing your PRs...</strong>
          <p className="review-prs__sync-banner-copy">
            We&apos;re loading your team assignments and PR data. This usually takes a minute or two on first login.
          </p>
        </div>
      )}

      {/* Requested by Me Section — PRs the user manually requested a review
          for (paste-a-URL form / API). Rendered only when non-empty; deleting
          an entry removes it and the section disappears with its last row. */}
      {hasRequestedPRs && (
        <section className="review-prs__requested-section">
          <div className="section-header">
            <h2>Requested by Me ({requestedPRs.length})</h2>
          </div>
          {isLoading && <LoadingSpinner />}
          {error && <ErrorMessage message={`Error loading PRs: ${error.message}`} />}
          {!isLoading && !error && requestedPRs.length > 0 && (
            <PRTable prs={requestedPRs} showViaTeams={false} />
          )}
        </section>
      )}

      {/* My PRs Section */}
      <section className="review-prs__my-section">
        <div className="section-header">
          <h2>My PRs ({myPRs.length})</h2>
        </div>
        {isLoading && <LoadingSpinner />}
        {error && <ErrorMessage message={`Error loading PRs: ${error.message}`} />}
        {!isLoading && !error && myPRs.length === 0 && (
          <p className="review-prs__empty-state">
            No PRs authored by you
          </p>
        )}
        {!isLoading && !error && myPRs.length > 0 && (
          <PRTable prs={myPRs} showViaTeams={false} />
        )}
      </section>

      {/* PRs to Review Section */}
      <section>
        <div className="section-header">
          <h2>PRs to Review ({reviewPRs.length})</h2>
        </div>
        {isLoading && <LoadingSpinner />}
        {error && <ErrorMessage message={`Error loading PRs: ${error.message}`} />}
        {!isLoading && !error && (
          <PRTable prs={reviewPRs} />
        )}
      </section>

      {/* Hidden Section — collapsed by default, only shown once the user has
          hidden something. Search/team/repo filters apply with the same
          semantics as the sections above (the count reflects matches). */}
      {hasHiddenPRs && (
        <section className="review-prs__hidden-section">
          <div className="section-header">
            <h2>
              <button
                type="button"
                className="hidden-section__toggle"
                aria-expanded={hiddenExpanded}
                onClick={() => setHiddenExpanded(v => !v)}
              >
                <span className="hidden-section__chevron" aria-hidden="true">
                  {hiddenExpanded ? '▾' : '▸'}
                </span>
                Hidden ({hiddenPRs.length})
              </button>
            </h2>
          </div>
          {hiddenExpanded && <PRTable prs={hiddenPRs} />}
        </section>
      )}
    </>
  );
}
