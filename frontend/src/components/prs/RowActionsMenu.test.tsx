import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { PR } from '@/types/pr';
import { RowActionsMenu } from './RowActionsMenu';

const useTelemetryMock = vi.fn();
vi.mock('@/hooks/useTelemetry', () => ({
  useTelemetry: () => useTelemetryMock(),
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

const renderMenu = (overrides: Partial<React.ComponentProps<typeof RowActionsMenu>> = {}) => {
  const props = {
    pr: makePR(),
    onTriggerReview: vi.fn(),
    onToggleHidden: vi.fn(),
    onDelete: vi.fn(),
    reviewPending: false,
    hiddenPending: false,
    deletePending: false,
    ...overrides,
  };
  render(<RowActionsMenu {...props} />);
  return props;
};

const openMenu = () => fireEvent.click(screen.getByRole('button', { name: /actions/i }));

describe('RowActionsMenu', () => {
  beforeEach(() => {
    useTelemetryMock.mockReturnValue({ track: vi.fn() });
  });
  afterEach(() => cleanup());

  it('hides the menu until the trigger is clicked', () => {
    renderMenu();
    expect(screen.queryByRole('menu')).toBeNull();
    openMenu();
    expect(screen.queryByRole('menu')).toBeTruthy();
    expect(screen.getByRole('menuitem', { name: /generate review/i })).toBeTruthy();
    expect(screen.getByRole('menuitem', { name: /delete/i })).toBeTruthy();
  });

  it('labels the review item "Generate" when no review exists', () => {
    renderMenu({ pr: makePR() });
    openMenu();
    expect(screen.getByRole('menuitem', { name: /generate review/i })).toBeTruthy();
    expect(screen.queryByRole('menuitem', { name: /regenerate/i })).toBeNull();
  });

  it('labels the review item "Regenerate" once a review exists', () => {
    renderMenu({ pr: makePR({ review_url: '/reviews/x.html', status: 'completed' }) });
    openMenu();
    expect(screen.getByRole('menuitem', { name: /regenerate review/i })).toBeTruthy();
  });

  it('calls onTriggerReview and closes the menu when Review is clicked', () => {
    const { onTriggerReview } = renderMenu();
    openMenu();
    fireEvent.click(screen.getByRole('menuitem', { name: /generate review/i }));
    expect(onTriggerReview).toHaveBeenCalledTimes(1);
    expect(screen.queryByRole('menu')).toBeNull();
  });

  it('calls onDelete when Delete is clicked', () => {
    const { onDelete } = renderMenu();
    openMenu();
    fireEvent.click(screen.getByRole('menuitem', { name: /delete/i }));
    expect(onDelete).toHaveBeenCalledTimes(1);
  });

  it('disables the review item and shows "Reviewing…" while a review is generating', () => {
    renderMenu({ pr: makePR({ status: 'generating' }) });
    openMenu();
    const item = screen.getByRole('menuitem', { name: /reviewing/i }) as HTMLButtonElement;
    expect(item.disabled).toBe(true);
  });

  it('never colors the kebab trigger by review status', () => {
    renderMenu({ pr: makePR({ status: 'agent_reviewing' }) });
    const trigger = screen.getByRole('button', { name: /actions/i });
    expect(trigger.className).toBe('row-actions__trigger');
    expect(trigger.getAttribute('title')).toBe('Row actions');
  });

  it('treats agent_reviewing as in-flight too', () => {
    renderMenu({ pr: makePR({ status: 'agent_reviewing' }) });
    openMenu();
    const item = screen.getByRole('menuitem', { name: /reviewing/i }) as HTMLButtonElement;
    expect(item.disabled).toBe(true);
  });

  it('shows a Hide item for a visible PR', () => {
    renderMenu();
    openMenu();
    expect(screen.getByRole('menuitem', { name: /hide/i })).toBeTruthy();
    expect(screen.queryByRole('menuitem', { name: /unhide/i })).toBeNull();
  });

  it('shows an Unhide item for a hidden PR', () => {
    renderMenu({ pr: makePR({ hidden: true }) });
    openMenu();
    expect(screen.getByRole('menuitem', { name: /unhide/i })).toBeTruthy();
  });

  it('calls onToggleHidden and closes the menu when Hide is clicked', () => {
    const { onToggleHidden } = renderMenu();
    openMenu();
    fireEvent.click(screen.getByRole('menuitem', { name: /hide/i }));
    expect(onToggleHidden).toHaveBeenCalledTimes(1);
    expect(screen.queryByRole('menu')).toBeNull();
  });

  it('disables the hide item and shows "Updating…" while the toggle is pending', () => {
    renderMenu({ hiddenPending: true });
    openMenu();
    const item = screen.getByRole('menuitem', { name: /updating/i }) as HTMLButtonElement;
    expect(item.disabled).toBe(true);
  });

  it('shows "Deleting…" and disables Delete while a delete is pending', () => {
    renderMenu({ deletePending: true });
    openMenu();
    const item = screen.getByRole('menuitem', { name: /deleting/i }) as HTMLButtonElement;
    expect(item.disabled).toBe(true);
  });

  it('reflects open state via aria-expanded and closes on Escape', () => {
    renderMenu();
    const trigger = screen.getByRole('button', { name: /actions/i });
    expect(trigger.getAttribute('aria-haspopup')).toBe('menu');
    expect(trigger.getAttribute('aria-expanded')).toBe('false');
    fireEvent.click(trigger);
    expect(trigger.getAttribute('aria-expanded')).toBe('true');
    fireEvent.keyDown(document, { key: 'Escape' });
    expect(screen.queryByRole('menu')).toBeNull();
    expect(trigger.getAttribute('aria-expanded')).toBe('false');
  });
});
