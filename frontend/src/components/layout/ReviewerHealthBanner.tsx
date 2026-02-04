import { useReviewerHealth } from '@/hooks/useReviewerHealth';
import './ReviewerHealthBanner.scss';

interface ReviewerHealthBannerProps {
  showReviewColumns: boolean;
  onToggleColumns: () => void;
}

export function ReviewerHealthBanner({ showReviewColumns, onToggleColumns }: ReviewerHealthBannerProps) {
  const { data: health, isLoading } = useReviewerHealth();

  if (isLoading || !health) {
    return null;
  }

  // Determine banner style based on health
  const bannerClass = health.healthy ? 'healthy' : 'error';

  return (
    <div className={`reviewer-health-banner ${bannerClass}`}>
      <div className="banner-content">
        <div className="banner-message">
          {!health.healthy && (
            <>
              <span className="banner-icon">⚠️</span>
              <span className="banner-text">
                <strong>Reviewer Issue:</strong> {health.reason}
              </span>
            </>
          )}
          {health.healthy && (
            <span className="banner-text">
              Review Columns
            </span>
          )}
        </div>
        <button
          className="toggle-columns-btn"
          onClick={onToggleColumns}
          title={showReviewColumns ? 'Hide review columns' : 'Show review columns'}
        >
          {showReviewColumns ? '👁️ Hide AI Reviews' : '👁️ Show AI Reviews'}
        </button>
      </div>
      {!health.healthy && (
        <div className="banner-details">
          <small>
            Total attempts: {health.attempted_count}
            {health.attempted_count > 0 && (
              <> | ✅ {health.success_count} | ❌ {health.error_count}</>
            )}
            {!health.reviewer_exists && <> | Reviewer not configured (GEMINI_API_KEY missing)</>}
          </small>
        </div>
      )}
    </div>
  );
}
