import { describe, it, expect } from 'vitest';
import { parseViaTeam, getTeamFilterName, buildViaTeamParts } from './teamFilters';
import { VIA_TEAMS_PERSONAL } from '@/constants';

describe('parseViaTeam', () => {
  it('splits "team:status" into name and status', () => {
    expect(parseViaTeam('backend:pending')).toEqual({ name: 'backend', status: 'pending' });
  });

  it('defaults to "pending" when there is no colon', () => {
    expect(parseViaTeam('backend')).toEqual({ name: 'backend', status: 'pending' });
  });

  it('treats the status as the suffix after the LAST colon', () => {
    // A team name containing a colon must stay intact.
    expect(parseViaTeam('group:sub:reviewing')).toEqual({ name: 'group:sub', status: 'reviewing' });
  });

  it('handles an empty status after a trailing colon', () => {
    expect(parseViaTeam('backend:')).toEqual({ name: 'backend', status: '' });
  });
});

describe('getTeamFilterName', () => {
  it('returns the team name without the status suffix', () => {
    expect(getTeamFilterName('backend:completed')).toBe('backend');
  });

  it('uses the last colon so colon-containing names survive', () => {
    expect(getTeamFilterName('group:sub:pending')).toBe('group:sub');
  });

  it('renders the personal sentinel as the username handle', () => {
    expect(getTeamFilterName(VIA_TEAMS_PERSONAL, 'octocat')).toBe('@octocat');
  });

  it('falls back to "@you" for the personal sentinel without a username', () => {
    expect(getTeamFilterName(VIA_TEAMS_PERSONAL)).toBe('@you');
  });
});

describe('buildViaTeamParts', () => {
  it('returns an empty array when there are no teams', () => {
    expect(buildViaTeamParts([])).toEqual([]);
  });

  it('parses each team entry', () => {
    expect(buildViaTeamParts(['backend:pending', 'frontend:reviewing'])).toEqual([
      { name: 'backend', status: 'pending' },
      { name: 'frontend', status: 'reviewing' },
    ]);
  });

  it('prepends a "@you" personal entry before the teams', () => {
    expect(buildViaTeamParts([VIA_TEAMS_PERSONAL, 'backend:pending'])).toEqual([
      { name: '@you', status: 'personal' },
      { name: 'backend', status: 'pending' },
    ]);
  });

  it('keeps the personal entry first regardless of its position in the input', () => {
    expect(buildViaTeamParts(['backend:pending', VIA_TEAMS_PERSONAL])).toEqual([
      { name: '@you', status: 'personal' },
      { name: 'backend', status: 'pending' },
    ]);
  });

  it('returns only the personal entry when that is the sole member', () => {
    expect(buildViaTeamParts([VIA_TEAMS_PERSONAL])).toEqual([
      { name: '@you', status: 'personal' },
    ]);
  });
});
