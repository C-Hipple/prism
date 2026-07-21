import { cleanup, render, screen } from '@testing-library/react';
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

describe('ReviewLinkMenu verdict badge', () => {
  beforeEach(() => {
    useTelemetryMock.mockReturnValue({ track: vi.fn() });
  });
  afterEach(() => cleanup());

  it('renders the red R badge when the verdict is request_changes', () => {
    render(
      <ReviewLinkMenu
        pr={makePR({ review_verdict: 'request_changes' })}
        reviewUrl="/reviews/review.html"
      />
    );
    const el = badge();
    expect(el).toBeTruthy();
    expect(el?.textContent).toBe('R');
    expect(el?.className).toContain('review-menu__verdict-badge');
    // The View link itself is unchanged next to the badge.
    expect(screen.getByRole('link', { name: /view/i })).toBeTruthy();
  });

  it('renders no badge for an approve verdict', () => {
    render(<ReviewLinkMenu pr={makePR({ review_verdict: 'approve' })} reviewUrl="/reviews/review.html" />);
    expect(badge()).toBeNull();
  });

  it('renders no badge for approve_suggestions', () => {
    render(
      <ReviewLinkMenu
        pr={makePR({ review_verdict: 'approve_suggestions' })}
        reviewUrl="/reviews/review.html"
      />
    );
    expect(badge()).toBeNull();
  });

  it('renders no badge when the verdict is empty', () => {
    render(<ReviewLinkMenu pr={makePR({ review_verdict: '' })} reviewUrl="/reviews/review.html" />);
    expect(badge()).toBeNull();
  });

  it('renders no badge when the verdict field is absent (older payloads)', () => {
    render(<ReviewLinkMenu pr={makePR()} reviewUrl="/reviews/review.html" />);
    expect(badge()).toBeNull();
  });

  it('renders no badge without a review URL even if the verdict is request_changes', () => {
    render(<ReviewLinkMenu pr={makePR({ review_verdict: 'request_changes' })} reviewUrl="" />);
    expect(badge()).toBeNull();
  });
});
