import type { PR } from '../../types/pr';

export type TriageFilter = 'critical' | 'medium' | 'quick-approve';

export function categorizePR(pr: PR): TriageFilter {
  // Critical: has CRITICAL issues
  if (pr.critical_count > 0) {
    return 'critical';
  }
  // Medium: has 2+ MEDIUM issues
  if (pr.medium_count >= 2) {
    return 'medium';
  }
  // Quick approve: only LOW or 0-1 MEDIUM issues
  return 'quick-approve';
}
