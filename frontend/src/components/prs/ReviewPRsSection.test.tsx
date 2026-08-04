import { cleanup, fireEvent, render, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { ReviewPRsSection } from './ReviewPRsSection';
import type { PR } from '@/types/pr';

const usePRsMock = vi.fn();
const useCurrentUserMock = vi.fn();
const useTelemetryMock = vi.fn();

vi.mock('@/hooks/usePRs', () => ({
  usePRs: () => usePRsMock(),
}));

vi.mock('@/hooks/useCurrentUser', () => ({
  useCurrentUser: () => useCurrentUserMock(),
}));

vi.mock('@/hooks/useTelemetry', () => ({
  useTelemetry: () => useTelemetryMock(),
}));

vi.mock('./PRTable', () => ({
  PRTable: ({ prs }: { prs: PR[] }) => (
    <div>
      <div>PR table</div>
      <div data-testid="pr-table-rows">{prs.map((pr) => pr.title).join('|')}</div>
    </div>
  ),
}));

vi.mock('@/components/common', () => ({
  LoadingSpinner: () => <div>Loading</div>,
  ErrorMessage: ({ message }: { message: string }) => <div>{message}</div>,
}));

vi.mock('./AutoReviewToggle', () => ({
  AutoReviewToggle: () => <button>Auto Review</button>,
}));

vi.mock('../filters', () => ({
  FilterBar: () => <div>Filter Bar</div>,
}));

vi.mock('./triageUtils', () => ({
  categorizePR: () => 'low',
}));

describe('ReviewPRsSection', () => {
  const makePR = (partial: Partial<PR>): PR => ({
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

  beforeEach(() => {
    usePRsMock.mockReturnValue({
      data: [],
      isLoading: false,
      error: null,
    });
    useCurrentUserMock.mockReturnValue({
      data: { github_username: 'alice' },
    });
  });

  afterEach(() => {
    cleanup();
  });

  it('tracks the review page view once on mount', async () => {
    const track = vi.fn();
    useTelemetryMock.mockReturnValue({ track });

    render(<ReviewPRsSection />);

    await waitFor(() => {
      expect(track).toHaveBeenCalledWith('view_review_prs_page');
    });

    expect(track).toHaveBeenCalledTimes(1);
  });

  it('applies search, team, and repo filters as an intersection for review PRs', () => {
    useTelemetryMock.mockReturnValue({ track: vi.fn() });
    usePRsMock.mockReturnValue({
      data: [
        makePR({
          number: 11,
          title: 'Viewer billing fix',
          repo: 'test-repo',
          via_teams: ['Viewer Team:pending'],
        }),
        makePR({
          number: 12,
          title: 'Viewer chat cleanup',
          repo: 'other-repo',
          via_teams: ['Viewer Team:pending'],
        }),
        makePR({
          number: 13,
          title: 'Creator billing fix',
          repo: 'test-repo',
          via_teams: ['Creator Team:pending'],
        }),
        makePR({
          number: 14,
          title: 'My own billing fix',
          repo: 'test-repo',
          via_teams: ['Viewer Team:pending'],
          is_mine: true,
        }),
      ],
      isLoading: false,
      error: null,
    });

    const { getByText, getAllByTestId } = render(
      <ReviewPRsSection
        searchTerm="billing"
        selectedTeams={['Viewer Team']}
        selectedRepos={['test-org/test-repo']}
      />
    );

    expect(getByText('PRs to Review (1)')).toBeTruthy();

    const tables = getAllByTestId('pr-table-rows');
    expect(tables[0].textContent).toContain('My own billing fix');
    expect(tables[1].textContent).toBe('Viewer billing fix');
  });

  it('filters by draft vs ready-for-review state', () => {
    useTelemetryMock.mockReturnValue({ track: vi.fn() });
    usePRsMock.mockReturnValue({
      data: [
        makePR({ number: 21, title: 'Ready review PR', draft: false }),
        makePR({ number: 22, title: 'Draft review PR', draft: true }),
        makePR({ number: 23, title: 'My draft PR', draft: true, is_mine: true }),
      ],
      isLoading: false,
      error: null,
    });

    // Draft only: applies to My PRs and PRs to Review alike.
    const draft = render(<ReviewPRsSection selectedStates={['draft']} />);
    expect(draft.getByText('My PRs (1)')).toBeTruthy();
    expect(draft.getByText('PRs to Review (1)')).toBeTruthy();
    expect(draft.getAllByTestId('pr-table-rows')[1].textContent).toBe('Draft review PR');
    cleanup();

    // Ready only.
    const ready = render(<ReviewPRsSection selectedStates={['ready']} />);
    expect(ready.getByText('PRs to Review (1)')).toBeTruthy();
    expect(ready.getAllByTestId('pr-table-rows')[0].textContent).toBe('Ready review PR');
    cleanup();

    // Both selected is equivalent to no state filter.
    const both = render(<ReviewPRsSection selectedStates={['draft', 'ready']} />);
    expect(both.getByText('PRs to Review (2)')).toBeTruthy();
  });

  it('matches personal team filters after username normalization', () => {
    useTelemetryMock.mockReturnValue({ track: vi.fn() });
    usePRsMock.mockReturnValue({
      data: [
        makePR({
          number: 21,
          title: 'Personally requested PR',
          via_teams: ['__PERSONAL__'],
        }),
        makePR({
          number: 22,
          title: 'Other team PR',
          via_teams: ['Viewer Team:pending'],
        }),
      ],
      isLoading: false,
      error: null,
    });

    const { getByText, getByTestId } = render(
      <ReviewPRsSection selectedTeams={['@alice']} />
    );

    expect(getByText('PRs to Review (1)')).toBeTruthy();
    expect(getByTestId('pr-table-rows').textContent).toContain('Personally requested PR');
  });

  it('renders no Hidden section when nothing is hidden', () => {
    useTelemetryMock.mockReturnValue({ track: vi.fn() });
    usePRsMock.mockReturnValue({
      data: [makePR({ number: 31, title: 'Visible PR' })],
      isLoading: false,
      error: null,
    });

    const { queryByText } = render(<ReviewPRsSection />);
    expect(queryByText(/^Hidden \(/)).toBeNull();
  });

  it('collects hidden PRs in a collapsed Hidden section and removes them from the other tables', () => {
    useTelemetryMock.mockReturnValue({ track: vi.fn() });
    usePRsMock.mockReturnValue({
      data: [
        makePR({ number: 41, title: 'Visible review PR' }),
        makePR({ number: 42, title: 'Hidden review PR', hidden: true }),
        makePR({ number: 43, title: 'Hidden own PR', hidden: true, is_mine: true }),
      ],
      isLoading: false,
      error: null,
    });

    const { getByText, getByRole, getAllByTestId } = render(<ReviewPRsSection />);

    expect(getByText('My PRs (0)')).toBeTruthy();
    expect(getByText('PRs to Review (1)')).toBeTruthy();

    // Collapsed by default: header with count visible, no hidden table yet.
    const toggle = getByRole('button', { name: /hidden \(2\)/i });
    expect(toggle.getAttribute('aria-expanded')).toBe('false');
    let tables = getAllByTestId('pr-table-rows');
    expect(tables).toHaveLength(1);
    expect(tables[0].textContent).toBe('Visible review PR');

    // Expanding reveals the hidden rows (mine and to-review alike).
    fireEvent.click(toggle);
    expect(toggle.getAttribute('aria-expanded')).toBe('true');
    tables = getAllByTestId('pr-table-rows');
    expect(tables).toHaveLength(2);
    expect(tables[1].textContent).toContain('Hidden review PR');
    expect(tables[1].textContent).toContain('Hidden own PR');

    // And it collapses again.
    fireEvent.click(toggle);
    expect(getAllByTestId('pr-table-rows')).toHaveLength(1);
  });

  it('renders no Requested by Me section when the user has no manual requests', () => {
    useTelemetryMock.mockReturnValue({ track: vi.fn() });
    usePRsMock.mockReturnValue({
      data: [makePR({ number: 61, title: 'Team-assigned PR' })],
      isLoading: false,
      error: null,
    });

    const { queryByText } = render(<ReviewPRsSection />);
    expect(queryByText(/^Requested by Me \(/)).toBeNull();
  });

  it('puts manually requested PRs in a top section and out of the other tables', () => {
    useTelemetryMock.mockReturnValue({ track: vi.fn() });
    usePRsMock.mockReturnValue({
      data: [
        makePR({ number: 71, title: 'Requested merged PR', via_manual: true }),
        makePR({ number: 72, title: 'Requested own PR', via_manual: true, is_mine: true }),
        makePR({ number: 73, title: 'My normal PR', is_mine: true }),
        makePR({ number: 74, title: 'Review PR' }),
      ],
      isLoading: false,
      error: null,
    });

    const { getByText, getByRole, getAllByTestId } = render(<ReviewPRsSection />);

    const toggle = getByRole('button', { name: /requested by me \(2\)/i });
    expect(toggle.getAttribute('aria-expanded')).toBe('true');
    expect(getByText('My PRs (1)')).toBeTruthy();
    expect(getByText('PRs to Review (1)')).toBeTruthy();

    let tables = getAllByTestId('pr-table-rows');
    expect(tables[0].textContent).toContain('Requested merged PR');
    expect(tables[0].textContent).toContain('Requested own PR');
    expect(tables[1].textContent).toBe('My normal PR');
    expect(tables[2].textContent).toBe('Review PR');

    // Collapsible like the Hidden section, but starts expanded.
    fireEvent.click(toggle);
    expect(toggle.getAttribute('aria-expanded')).toBe('false');
    tables = getAllByTestId('pr-table-rows');
    expect(tables).toHaveLength(2);
    expect(tables[0].textContent).toBe('My normal PR');
  });

  it('shows an empty state when filters exclude all requested PRs', () => {
    useTelemetryMock.mockReturnValue({ track: vi.fn() });
    usePRsMock.mockReturnValue({
      data: [
        makePR({ number: 91, title: 'Requested chat PR', via_manual: true }),
        makePR({ number: 92, title: 'Billing review PR' }),
      ],
      isLoading: false,
      error: null,
    });

    const { getByText, getByRole } = render(<ReviewPRsSection searchTerm="billing" />);
    expect(getByRole('button', { name: /requested by me \(0\)/i })).toBeTruthy();
    expect(getByText('No matching requested PRs')).toBeTruthy();
  });

  it('moves a hidden manually requested PR to the Hidden section, not Requested by Me', () => {
    useTelemetryMock.mockReturnValue({ track: vi.fn() });
    usePRsMock.mockReturnValue({
      data: [
        makePR({ number: 81, title: 'Deleted requested PR', via_manual: true, hidden: true }),
      ],
      isLoading: false,
      error: null,
    });

    const { queryByText, getByRole } = render(<ReviewPRsSection />);
    expect(queryByText(/^Requested by Me \(/)).toBeNull();
    expect(getByRole('button', { name: /hidden \(1\)/i })).toBeTruthy();
  });

  it('applies the search term to the Hidden section with the same semantics', () => {
    useTelemetryMock.mockReturnValue({ track: vi.fn() });
    usePRsMock.mockReturnValue({
      data: [
        makePR({ number: 51, title: 'Visible billing PR' }),
        makePR({ number: 52, title: 'Hidden billing PR', hidden: true }),
        makePR({ number: 53, title: 'Hidden chat PR', hidden: true }),
      ],
      isLoading: false,
      error: null,
    });

    const { getByRole, getAllByTestId } = render(<ReviewPRsSection searchTerm="billing" />);

    const toggle = getByRole('button', { name: /hidden \(1\)/i });
    fireEvent.click(toggle);
    const tables = getAllByTestId('pr-table-rows');
    expect(tables[1].textContent).toBe('Hidden billing PR');
  });
});
