import { describe, expect, it } from 'vitest';
import type { PR } from '@/types/pr';
import { applyPRWebSocketMessage } from './websocketCacheUpdates';
import type { ServerWebSocketMessage } from './websocket';

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

const prUpdated = (payload: PR): ServerWebSocketMessage => ({
  type: 'pr_updated',
  payload,
});

describe('applyPRWebSocketMessage hidden handling', () => {
  it('keeps a hidden row hidden when an update payload carries hidden=true', () => {
    const oldData = [makePR({ hidden: true })];
    const result = applyPRWebSocketMessage(oldData, prUpdated(makePR({ status: 'completed', hidden: true })));
    expect(result?.[0].hidden).toBe(true);
    expect(result?.[0].status).toBe('completed');
  });

  it('un-hides a row when the server says hidden=false (e.g. unhidden in another tab)', () => {
    const oldData = [makePR({ hidden: true })];
    const result = applyPRWebSocketMessage(oldData, prUpdated(makePR({ hidden: false })));
    expect(result?.[0].hidden).toBe(false);
  });

  it('preserves the local hidden flag when an older payload omits the field', () => {
    const oldData = [makePR({ hidden: true })];
    const payload = makePR();
    delete payload.hidden;
    const result = applyPRWebSocketMessage(oldData, prUpdated(payload));
    expect(result?.[0].hidden).toBe(true);
  });
});
