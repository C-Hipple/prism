import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { PR } from '@/types/pr';
import { PRTableRow } from './PRTableRow';

const { triggerMutate, deleteMutate } = vi.hoisted(() => ({
  triggerMutate: vi.fn(),
  deleteMutate: vi.fn(),
}));

vi.mock('@/hooks/usePRs', () => ({
  useDeletePR: () => ({ mutate: deleteMutate, isPending: false }),
  useTriggerReview: () => ({ mutate: triggerMutate, isPending: false }),
}));
vi.mock('@/hooks/useTelemetry', () => ({
  useTelemetry: () => ({ track: vi.fn() }),
}));
vi.mock('@/components/common', () => ({
  CommitSha: () => <span>sha</span>,
}));
vi.mock('./CIStatusIndicator', () => ({ CIStatusIndicator: () => <span>ci</span> }));
vi.mock('./NotesCell', () => ({ NotesCell: () => <span>notes</span> }));
vi.mock('./RowActionsMenu', () => ({ RowActionsMenu: () => <span data-testid="actions" /> }));
vi.mock('./ReviewLinkMenu', () => ({
  ReviewLinkMenu: () => <span data-testid="review-link-menu" />,
}));

const makePR = (partial: Partial<PR> = {}): PR => ({
  owner: 'test-org',
  repo: 'test-repo',
  number: 1,
  commit_sha: 'abc123',
  last_reviewed_at: null,
  review_html_path: '',
  github_url: 'https://github.com/test-org/test-repo/pull/1',
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

const renderRow = (pr: PR) =>
  render(
    <table>
      <tbody>
        <PRTableRow pr={pr} />
      </tbody>
    </table>
  );

describe('PRTableRow review cell', () => {
  beforeEach(() => vi.clearAllMocks());
  afterEach(() => cleanup());

  it('shows "Generating…" in yellow (gemini) while a Gemini review runs', () => {
    renderRow(makePR({ status: 'generating' }));
    const el = screen.getByText(/Generating/);
    expect(el.className).toContain('pr-table__review-generating--gemini');
    expect(screen.queryByTestId('review-link-menu')).toBeNull();
  });

  it('shows "Generating…" in orange (agent) during the agent pass', () => {
    renderRow(makePR({ status: 'agent_reviewing' }));
    const el = screen.getByText(/Generating/);
    expect(el.className).toContain('pr-table__review-generating--agent');
  });

  it('renders the review link menu when a completed review exists', () => {
    renderRow(makePR({ status: 'completed', review_url: '/reviews/x.html' }));
    expect(screen.queryByTestId('review-link-menu')).toBeTruthy();
    expect(screen.queryByText(/Generating/)).toBeNull();
  });

  it('shows ERROR when the review failed', () => {
    renderRow(makePR({ status: 'error', error_message: 'boom' }));
    expect(screen.getByText('ERROR')).toBeTruthy();
    expect(screen.queryByText(/Generating/)).toBeNull();
  });

  it('shows a Generate button when a PR has no review yet', () => {
    renderRow(makePR({ status: 'pending', review_url: '' }));
    const btn = screen.getByRole('button', { name: /generate/i });
    expect(btn).toBeTruthy();
    expect(screen.queryByTestId('review-link-menu')).toBeNull();
    expect(screen.queryByText(/Generating/)).toBeNull();
  });

  it('shows Generate (not the review menu) once a stale review has been cleared to pending', () => {
    // A new commit resets the PR to pending and clears review_url server-side.
    renderRow(makePR({ status: 'pending', review_url: '', last_reviewed_at: null }));
    expect(screen.getByRole('button', { name: /generate/i })).toBeTruthy();
    expect(screen.queryByTestId('review-link-menu')).toBeNull();
  });

  it('triggers a review when Generate is clicked', () => {
    const pr = makePR({ status: 'pending', review_url: '' });
    renderRow(pr);
    fireEvent.click(screen.getByRole('button', { name: /generate/i }));
    expect(triggerMutate).toHaveBeenCalledTimes(1);
    expect(triggerMutate).toHaveBeenCalledWith({ owner: pr.owner, repo: pr.repo, number: pr.number });
  });

  it('does not show Generate while a review is generating', () => {
    renderRow(makePR({ status: 'generating' }));
    expect(screen.queryByRole('button', { name: /generate/i })).toBeNull();
  });

  it('does not show Generate while the agent pass is running', () => {
    renderRow(makePR({ status: 'agent_reviewing' }));
    expect(screen.queryByRole('button', { name: /generate/i })).toBeNull();
  });

  it('does not show Generate when an up-to-date review exists', () => {
    renderRow(makePR({ status: 'completed', review_url: '/reviews/x.html' }));
    expect(screen.queryByRole('button', { name: /generate/i })).toBeNull();
    expect(screen.queryByTestId('review-link-menu')).toBeTruthy();
  });

  it('does not show Generate when the review errored (ERROR shown instead)', () => {
    renderRow(makePR({ status: 'error', error_message: 'boom' }));
    expect(screen.queryByRole('button', { name: /generate/i })).toBeNull();
    expect(screen.getByText('ERROR')).toBeTruthy();
  });

  it('falls back to Generate when status is completed but no review_url is present', () => {
    renderRow(makePR({ status: 'completed', review_url: '' }));
    expect(screen.getByRole('button', { name: /generate/i })).toBeTruthy();
  });
});
