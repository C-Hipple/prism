import { describe, expect, it, vi } from 'vitest';

const apiPostMock = vi.fn();
vi.mock('./client', () => ({
  apiPost: (path: string, body: unknown) => apiPostMock(path, body),
}));

import { generateReview, triggerReview } from './prs';

describe('generateReview', () => {
  it('marks dashboard submissions with source=form so the server claims the PR', async () => {
    apiPostMock.mockResolvedValue({ status: 'success' });
    await generateReview({ owner: 'acme', repo: 'example', number: 7 });
    expect(apiPostMock).toHaveBeenCalledWith('/api/prs/generate-review', {
      owner: 'acme',
      repo: 'example',
      number: 7,
      source: 'form',
    });
  });

  it('triggerReview stays unmarked', async () => {
    apiPostMock.mockResolvedValue({ status: 'success' });
    await triggerReview({ owner: 'acme', repo: 'example', number: 7 });
    expect(apiPostMock).toHaveBeenCalledWith('/api/prs/trigger-review', {
      owner: 'acme',
      repo: 'example',
      number: 7,
    });
  });
});
