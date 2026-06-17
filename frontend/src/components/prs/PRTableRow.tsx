import { memo, useCallback, type MouseEvent } from 'react';
import type { PR } from '@/types/pr';
import { CommitSha } from '@/components/common';
import { useDeletePR, useTriggerReview } from '@/hooks/usePRs';
import { useTelemetry } from '@/hooks/useTelemetry';
import { CIStatusIndicator } from './CIStatusIndicator';
import { NotesCell } from './NotesCell';
import { ReviewLinkMenu } from './ReviewLinkMenu';
import { RowActionsMenu } from './RowActionsMenu';
import { buildViaTeamParts } from '@/utils/teamFilters';

interface PRTableRowProps {
  pr: PR;
  showViaTeams?: boolean;
}

export const PRTableRow = memo(function PRTableRow({
  pr,
  showViaTeams = true
}: PRTableRowProps) {
  const deleteMutation = useDeletePR();
  const triggerReviewMutation = useTriggerReview();
  const { track } = useTelemetry();
  const prUrl = `https://github.com/${pr.owner}/${pr.repo}/pull/${pr.number}`;
  const reviewUrl = pr.status === 'completed' && pr.review_url
    ? pr.review_url
    : null;

  const handleDelete = useCallback(() => {
    track('delete_pr', { pr_owner: pr.owner, pr_repo: pr.repo, pr_number: pr.number });
    deleteMutation.mutate({
      owner: pr.owner,
      repo: pr.repo,
      number: pr.number,
    });
  }, [pr.owner, pr.repo, pr.number, deleteMutation, track]);

  const handleTriggerReview = useCallback(() => {
    track('trigger_review', { pr_owner: pr.owner, pr_repo: pr.repo, pr_number: pr.number });
    triggerReviewMutation.mutate({
      owner: pr.owner,
      repo: pr.repo,
      number: pr.number,
    });
  }, [pr.owner, pr.repo, pr.number, triggerReviewMutation, track]);

  const handleOpenPr = useCallback((e: MouseEvent<HTMLAnchorElement>) => {
    track('open_pr_github', { pr_owner: pr.owner, pr_repo: pr.repo, pr_number: pr.number });
    // Opt-in same-tab: Alt/Option+click navigates the current tab instead of
    // opening a new one. Plain click (and Ctrl/Cmd/Shift/middle-click) keep the
    // browser's default new-tab behavior via target="_blank".
    if (e.altKey) {
      e.preventDefault();
      window.location.assign(prUrl);
    }
  }, [pr.owner, pr.repo, pr.number, prUrl, track]);

  return (
    <tr>
      <td>
        <a href={prUrl} target="_blank" rel="noopener noreferrer" title="Alt/Option-click to open in this tab" onClick={handleOpenPr}>
          {pr.owner}/{pr.repo} #{pr.number}
        </a>
        {pr.draft && <span className="pr-table__draft-indicator"> (Draft)</span>}
        <div className="pr-table__title">{pr.title}</div>
      </td>
      <td>{pr.author}</td>
      <td>
        <CommitSha sha={pr.commit_sha} owner={pr.owner} repo={pr.repo} />
      </td>
      <td className="pr-table__ci-status">
        <CIStatusIndicator state={pr.ci_state} failedChecks={pr.ci_failed_checks} />
      </td>
      <td className={`pr-table__approval-count ${pr.approval_count > 0 ? 'pr-table__approval-count--positive' : 'pr-table__approval-count--zero'}`}>
        {pr.approval_count}
      </td>
      <td className="pr-table__my-review">
        {pr.my_review_status === 'APPROVED' && <span className="pr-table__my-review--approved" title="You approved this PR">✓</span>}
        {pr.my_review_status === 'CHANGES_REQUESTED' && <span className="pr-table__my-review--changes" title="You requested changes">✗</span>}
        {pr.my_review_status === 'COMMENTED' && <span className="pr-table__my-review--commented" title="You commented">💬</span>}
        {!pr.my_review_status && <span className="pr-table__my-review--none" title="No review yet">-</span>}
      </td>
      {showViaTeams && (
        <td className="pr-table__via-teams">
          {pr.via_teams && pr.via_teams.length > 0 ? (
            (() => {
              const parts = buildViaTeamParts(pr.via_teams);
              return (
                <span title={parts.map(p => `${p.name} (${p.status})`).join(', ')}>
                  {parts.map((p, i) => (
                    <span key={i}>
                      {i > 0 && ', '}
                      <span className={`pr-table__via-teams--${p.status}`}>{p.name}</span>
                    </span>
                  ))}
                </span>
              );
            })()
          ) : (
            <span className="pr-table__via-teams--none" title="Via team (auto-assigned)">-</span>
          )}
        </td>
      )}
      <td>
        <NotesCell
          owner={pr.owner}
          repo={pr.repo}
          number={pr.number}
          notes={pr.notes || ''}
        />
      </td>
      <td className="pr-table__review-cell">
        {pr.status === 'error' ? (
          <span className="pr-table__review-error" title={pr.error_message || 'Review failed'}>
            ERROR
          </span>
        ) : pr.status === 'generating' || pr.status === 'agent_reviewing' ? (
          <span
            className={`pr-table__review-generating pr-table__review-generating--${
              pr.status === 'agent_reviewing' ? 'agent' : 'gemini'
            }`}
            title="AI review in progress"
          >
            Generating…
          </span>
        ) : reviewUrl ? (
          <ReviewLinkMenu pr={pr} reviewUrl={reviewUrl} />
        ) : (
          // No up-to-date review: brand-new PR, or one whose prior review was
          // cleared server-side after a new commit made it stale.
          <button
            type="button"
            className="pr-table__generate-btn"
            onClick={handleTriggerReview}
            disabled={triggerReviewMutation.isPending}
            title="Generate an AI review for this PR"
          >
            {triggerReviewMutation.isPending ? 'Starting…' : '🔄 Generate'}
          </button>
        )}
      </td>
      <td>
        <RowActionsMenu
          pr={pr}
          onTriggerReview={handleTriggerReview}
          onDelete={handleDelete}
          reviewPending={triggerReviewMutation.isPending}
          deletePending={deleteMutation.isPending}
        />
      </td>
    </tr>
  );
});
