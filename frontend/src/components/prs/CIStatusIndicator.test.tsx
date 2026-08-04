import { cleanup, render } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import { CIStatusIndicator } from './CIStatusIndicator';

describe('CIStatusIndicator', () => {
  afterEach(() => {
    cleanup();
  });

  it('renders the CI state for open PRs', () => {
    const { container } = render(
      <CIStatusIndicator state="failure" failedChecks={['lint']} prState="open" />
    );
    const el = container.querySelector('.ci-status--failure');
    expect(el).toBeTruthy();
    expect(el?.getAttribute('title')).toContain('lint');
  });

  it('replaces a stale CI dot with the merged indicator', () => {
    const { container } = render(
      <CIStatusIndicator state="failure" failedChecks={['lint']} prState="merged" />
    );
    expect(container.querySelector('.ci-status--merged')).toBeTruthy();
    expect(container.querySelector('.ci-status--failure')).toBeNull();
    expect(container.querySelector('.ci-status')?.getAttribute('title')).toBe('PR merged');
  });

  it('shows closed-without-merging distinctly', () => {
    const { container } = render(
      <CIStatusIndicator state="success" failedChecks={[]} prState="closed" />
    );
    expect(container.querySelector('.ci-status--closed')).toBeTruthy();
  });

  it('treats a missing pr_state as open', () => {
    const { container } = render(<CIStatusIndicator state="success" failedChecks={[]} />);
    expect(container.querySelector('.ci-status--success')).toBeTruthy();
  });
});
