import { memo } from 'react';

interface CIStatusIndicatorProps {
  state: 'success' | 'failure' | 'pending' | 'unknown';
  failedChecks: string[];
  // GitHub PR state; 'merged'/'closed' replace the CI dot, whose last-known
  // state is stale and meaningless once the PR is no longer open.
  prState?: 'open' | 'closed' | 'merged';
}

export const CIStatusIndicator = memo(function CIStatusIndicator({ state, failedChecks, prState }: CIStatusIndicatorProps) {
  if (prState === 'merged' || prState === 'closed') {
    return (
      <span
        className={`ci-status ci-status--${prState}`}
        title={prState === 'merged' ? 'PR merged' : 'PR closed without merging'}
      >
        ●
      </span>
    );
  }

  const getIcon = () => {
    switch (state) {
      case 'success':
        return '●'; // Green circle
      case 'failure':
        return '●'; // Red circle
      case 'pending':
        return '●'; // Yellow circle
      default:
        return '○'; // Gray circle outline
    }
  };

  const getTitle = () => {
    if (state === 'failure' && failedChecks.length > 0) {
      return `Failed checks:\n${failedChecks.join('\n')}`;
    }
    switch (state) {
      case 'success':
        return 'All checks passed';
      case 'failure':
        return 'Some checks failed';
      case 'pending':
        return 'Checks in progress';
      default:
        return 'No CI checks';
    }
  };

  return (
    <span
      className={`ci-status ci-status--${state}`}
      title={getTitle()}
    >
      {getIcon()}
    </span>
  );
});
