import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { PR } from '@/types/pr';
import type { FindingOutcome, ReviewFinding } from '@/api/findingOutcomes';
import { CriticalFindingsTriage } from './CriticalFindingsTriage';

const useTelemetryMock = vi.fn();
vi.mock('@/hooks/useTelemetry', () => ({
  useTelemetry: () => useTelemetryMock(),
}));

const fetchFindingOutcomesMock = vi.fn();
const recordFindingOutcomeMock = vi.fn();
const fetchCriticalFindingsMock = vi.fn();
vi.mock('@/api/findingOutcomes', () => ({
  fetchFindingOutcomes: (...args: unknown[]) => fetchFindingOutcomesMock(...args),
  recordFindingOutcome: (...args: unknown[]) => recordFindingOutcomeMock(...args),
  fetchCriticalFindings: (...args: unknown[]) => fetchCriticalFindingsMock(...args),
}));

const makePR = (partial: Partial<PR> = {}): PR => ({
  owner: 'test-org',
  repo: 'test-repo',
  number: 1,
  commit_sha: 'abc1234def',
  last_reviewed_at: '2026-04-15T12:00:00Z',
  review_html_path: 'review.html',
  github_url: 'https://github.com/test-org/test-repo/pull/1',
  review_url: '/reviews/review.html',
  status: 'completed',
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
  critical_count: 1,
  medium_count: 0,
  low_count: 0,
  notes: '',
  ...partial,
});

const criticalFinding: ReviewFinding = {
  severity: 'critical',
  provenance: 'agent',
  file: 'pkg/api/handler.go',
  line: 42,
  comment: 'Nil map write: sessions[id] is assigned before the map is initialized',
};

const existingOutcome: FindingOutcome = {
  owner: 'test-org',
  repo: 'test-repo',
  number: 1,
  reviewed_sha: 'abc1234',
  fingerprint: 'pkg/api/handler.go:4:deadbeef0123',
  severity: 'critical',
  outcome: 'dismissed',
  reason: 'guarded upstream',
  decided_by: 'alice',
  decided_at: '2026-07-01T00:00:00Z',
};

const triageToggle = () => screen.queryByRole('menuitem', { name: /review findings/i });

const expandTriage = async () => {
  await waitFor(() => expect(triageToggle()).toBeTruthy());
  fireEvent.click(triageToggle()!);
};

describe('CriticalFindingsTriage', () => {
  beforeEach(() => {
    useTelemetryMock.mockReturnValue({ track: vi.fn() });
    fetchFindingOutcomesMock.mockResolvedValue([]);
    fetchCriticalFindingsMock.mockResolvedValue({ sha: 'abc1234', findings: [criticalFinding] });
    recordFindingOutcomeMock.mockResolvedValue({ status: 'success', outcome: existingOutcome });
  });
  afterEach(() => {
    cleanup();
    vi.clearAllMocks();
  });

  it('renders nothing when the outcomes endpoint is unavailable (feature off)', async () => {
    fetchFindingOutcomesMock.mockRejectedValue(new Error('404'));
    const { container } = render(<CriticalFindingsTriage pr={makePR()} />);
    await waitFor(() => expect(fetchFindingOutcomesMock).toHaveBeenCalled());
    expect(container.innerHTML).toBe('');
  });

  it('shows the toggle when enabled and lists criticals with dismiss/ack actions on expand', async () => {
    render(<CriticalFindingsTriage pr={makePR()} />);
    await expandTriage();

    await waitFor(() => expect(screen.getByText(/pkg\/api\/handler\.go:42/)).toBeTruthy());
    expect(screen.getByText(/nil map write/i)).toBeTruthy();
    expect(screen.getByRole('button', { name: /acknowledge/i })).toBeTruthy();
    expect(screen.getByRole('button', { name: /dismiss…/i })).toBeTruthy();
    expect(fetchCriticalFindingsMock).toHaveBeenCalledWith('test-org', 'test-repo', 1);
  });

  it('records an acknowledge for the finding against the reviewed SHA', async () => {
    render(<CriticalFindingsTriage pr={makePR()} />);
    await expandTriage();

    await waitFor(() => expect(screen.getByRole('button', { name: /acknowledge/i })).toBeTruthy());
    fireEvent.click(screen.getByRole('button', { name: /acknowledge/i }));

    await waitFor(() =>
      expect(recordFindingOutcomeMock).toHaveBeenCalledWith(
        expect.objectContaining({
          owner: 'test-org',
          repo: 'test-repo',
          number: 1,
          sha: 'abc1234', // review's SHA, not the PR's HEAD
          file: 'pkg/api/handler.go',
          line: 42,
          severity: 'critical',
          outcome: 'acknowledged',
        })
      )
    );
  });

  it('requires a reason before a dismiss can be submitted', async () => {
    render(<CriticalFindingsTriage pr={makePR()} />);
    await expandTriage();

    await waitFor(() => expect(screen.getByRole('button', { name: /dismiss…/i })).toBeTruthy());
    fireEvent.click(screen.getByRole('button', { name: /dismiss…/i }));

    const input = screen.getByLabelText(/dismiss reason/i) as HTMLInputElement;
    const confirm = screen.getByRole('button', { name: /^dismiss$/i }) as HTMLButtonElement;
    expect(confirm.disabled).toBe(true);

    fireEvent.change(input, { target: { value: 'guarded upstream' } });
    expect(confirm.disabled).toBe(false);
    fireEvent.click(confirm);

    await waitFor(() =>
      expect(recordFindingOutcomeMock).toHaveBeenCalledWith(
        expect.objectContaining({ outcome: 'dismissed', reason: 'guarded upstream' })
      )
    );
  });

  it('shows the recorded decision instead of buttons for an already-triaged finding', async () => {
    fetchFindingOutcomesMock.mockResolvedValue([existingOutcome]);
    render(<CriticalFindingsTriage pr={makePR()} />);
    await expandTriage();

    await waitFor(() => expect(screen.getByText(/dismissed by alice/i)).toBeTruthy());
    expect(screen.getByText(/guarded upstream/)).toBeTruthy();
    expect(screen.queryByRole('button', { name: /acknowledge/i })).toBeNull();
    expect(screen.queryByRole('button', { name: /dismiss/i })).toBeNull();
  });

  it('ignores an outcome recorded against an older review SHA, even in the same line bucket', async () => {
    // Same file + line bucket as the current finding, but decided on a prior
    // review — the newer, different finding must still be triageable.
    fetchFindingOutcomesMock.mockResolvedValue([
      { ...existingOutcome, reviewed_sha: '0ld5ha' },
    ]);
    render(<CriticalFindingsTriage pr={makePR()} />);
    await expandTriage();

    await waitFor(() => expect(screen.getByRole('button', { name: /acknowledge/i })).toBeTruthy());
    expect(screen.queryByText(/dismissed by alice/i)).toBeNull();
    expect(screen.getByRole('button', { name: /dismiss…/i })).toBeTruthy();
  });
});
