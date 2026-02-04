export interface PR {
  owner: string;
  repo: string;
  number: number;
  commit_sha: string;
  last_reviewed_at: string | null;
  review_html_path: string;
  github_url: string;
  review_url: string;
  status: 'pending' | 'generating' | 'completed' | 'error';
  title: string;
  author: string;
  generating_since: string | null;
  approval_count: number;
  my_review_status: 'APPROVED' | 'CHANGES_REQUESTED' | 'COMMENTED' | '';
  draft: boolean;
  ci_state: 'success' | 'failure' | 'pending' | 'unknown';
  ci_failed_checks: string[];
  created_at: string | null;
  is_mine: boolean;
  // Review importance counts
  critical_count: number;
  medium_count: number;
  low_count: number;
  // User notes
  notes: string;
}
