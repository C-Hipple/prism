import { act, cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { PR } from '@/types/pr';
import { ReviewLinkMenu } from './ReviewLinkMenu';

const useTelemetryMock = vi.fn();
vi.mock('@/hooks/useTelemetry', () => ({
  useTelemetry: () => useTelemetryMock(),
}));

const makePR = (partial: Partial<PR> = {}): PR => ({
  owner: 'test-org',
  repo: 'test-repo',
  number: 1,
  commit_sha: 'abc123',
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
  critical_count: 0,
  medium_count: 0,
  low_count: 0,
  notes: '',
  ...partial,
});

const badge = () => screen.queryByRole('img', { name: 'Verdict: request changes' });

const renderMenu = (overrides: Partial<React.ComponentProps<typeof ReviewLinkMenu>> = {}) => {
  const props = {
    pr: makePR(),
    reviewUrl: '/reviews/review.html',
    onTriggerReview: vi.fn(),
    reviewPending: false,
    ...overrides,
  };
  render(<ReviewLinkMenu {...props} />);
  return props;
};

describe('ReviewLinkMenu verdict badge', () => {
  beforeEach(() => {
    useTelemetryMock.mockReturnValue({ track: vi.fn() });
  });
  afterEach(() => cleanup());

  it('renders the red R badge when the verdict is request_changes', () => {
    renderMenu({ pr: makePR({ review_verdict: 'request_changes' }) });
    const el = badge();
    expect(el).toBeTruthy();
    expect(el?.textContent).toBe('R');
    expect(el?.className).toContain('review-menu__verdict-badge');
    // The View link itself is unchanged next to the badge.
    expect(screen.getByRole('link', { name: /view/i })).toBeTruthy();
  });

  it('renders no badge for an approve verdict', () => {
    renderMenu({ pr: makePR({ review_verdict: 'approve' }) });
    expect(badge()).toBeNull();
  });

  it('renders no badge for approve_suggestions', () => {
    renderMenu({ pr: makePR({ review_verdict: 'approve_suggestions' }) });
    expect(badge()).toBeNull();
  });

  it('renders no badge when the verdict is empty', () => {
    renderMenu({ pr: makePR({ review_verdict: '' }) });
    expect(badge()).toBeNull();
  });

  it('renders no badge when the verdict field is absent (older payloads)', () => {
    renderMenu({ pr: makePR() });
    expect(badge()).toBeNull();
  });

  it('renders no badge without a review URL even if the verdict is request_changes', () => {
    renderMenu({ pr: makePR({ review_verdict: 'request_changes' }), reviewUrl: '' });
    expect(badge()).toBeNull();
  });
});

describe('ReviewLinkMenu regenerate action', () => {
  beforeEach(() => {
    useTelemetryMock.mockReturnValue({ track: vi.fn() });
    vi.useFakeTimers();
  });
  afterEach(() => {
    cleanup();
    vi.useRealTimers();
  });

  // The panel opens off hover timers on the anchor span wrapping the View link.
  const openPanel = () => {
    const anchor = screen.getByRole('link', { name: /view/i }).parentElement!;
    fireEvent.mouseEnter(anchor);
    act(() => {
      vi.advanceTimersByTime(300);
    });
  };

  it('exposes a Regenerate review item in the hover panel', () => {
    renderMenu();
    openPanel();
    expect(screen.getByRole('menuitem', { name: /regenerate review/i })).toBeTruthy();
  });

  it('calls onTriggerReview and closes the panel when clicked', () => {
    const { onTriggerReview } = renderMenu();
    openPanel();
    fireEvent.click(screen.getByRole('menuitem', { name: /regenerate review/i }));
    expect(onTriggerReview).toHaveBeenCalledTimes(1);
    expect(screen.queryByRole('menu')).toBeNull();
  });

  it('disables the item and shows "Reviewing…" while the trigger is pending', () => {
    renderMenu({ reviewPending: true });
    openPanel();
    const item = screen.getByRole('menuitem', { name: /reviewing/i }) as HTMLButtonElement;
    expect(item.disabled).toBe(true);
    expect(screen.queryByRole('menuitem', { name: /regenerate review/i })).toBeNull();
  });
});
