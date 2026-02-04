import { useQuery } from '@tanstack/react-query';
import type { ReviewerHealth } from '@/types/reviewerHealth';
import { HEALTH_POLL_INTERVAL } from '@/utils/constants';

async function fetchReviewerHealth(): Promise<ReviewerHealth> {
  const response = await fetch('/api/reviewer-health');
  if (!response.ok) {
    throw new Error('Failed to fetch reviewer health');
  }
  return response.json();
}

export function useReviewerHealth() {
  return useQuery<ReviewerHealth>({
    queryKey: ['reviewer-health'],
    queryFn: fetchReviewerHealth,
    refetchInterval: HEALTH_POLL_INTERVAL > 0 ? HEALTH_POLL_INTERVAL : false,
    staleTime: 30000, // Consider data stale after 30 seconds
  });
}
