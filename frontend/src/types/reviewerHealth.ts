export interface ReviewerHealth {
  healthy: boolean;
  reason: string;
  recommendation: 'show_columns' | 'hide_columns';
  error_count: number;
  success_count: number;
  attempted_count: number;
  reviewer_exists: boolean;
  reviewer_path: string;
}
