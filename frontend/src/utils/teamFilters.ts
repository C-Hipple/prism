import { VIA_TEAMS_PERSONAL } from '@/constants';

/** A parsed via_team entry: the team (or display) name and its review status. */
export interface ViaTeamEntry {
  name: string;
  status: string;
}

/**
 * Splits a raw via_team string ("team:status") into its name and status.
 *
 * The status is always the suffix after the LAST colon, matching how the
 * backend builds these strings (`team + ":" + status`). Using the last
 * colon keeps the team name intact should one ever contain a colon. Strings
 * with no colon fall back to a "pending" status.
 */
export function parseViaTeam(team: string): ViaTeamEntry {
  const suffixIndex = team.lastIndexOf(':');
  if (suffixIndex === -1) {
    return { name: team, status: 'pending' };
  }
  return { name: team.slice(0, suffixIndex), status: team.slice(suffixIndex + 1) };
}

/**
 * Display name for a via_team string, used by the team filters. The personal
 * sentinel renders as the current user's handle (or "@you" when unknown).
 */
export function getTeamFilterName(team: string, username?: string): string {
  if (team === VIA_TEAMS_PERSONAL) {
    return username ? `@${username}` : '@you';
  }
  return parseViaTeam(team).name;
}

/**
 * Builds the ordered list of via_team entries for display in a PR row:
 * a leading "@you" entry (status "personal") when the PR was personally
 * requested, followed by each team with its parsed status. Returns an empty
 * array when there are no via_teams.
 */
export function buildViaTeamParts(viaTeams: string[]): ViaTeamEntry[] {
  const hasPersonal = viaTeams.includes(VIA_TEAMS_PERSONAL);
  const teamEntries = viaTeams
    .filter(t => t !== VIA_TEAMS_PERSONAL)
    .map(parseViaTeam);
  return [
    ...(hasPersonal ? [{ name: '@you', status: 'personal' }] : []),
    ...teamEntries,
  ];
}
