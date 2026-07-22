import { apiGet, apiPost } from './client';
import type { PR } from '@/types/pr';

export async function fetchPRs(): Promise<PR[]> {
  return apiGet<PR[]>('/api/prs');
}

export interface DeletePRParams {
  owner: string;
  repo: string;
  number: number;
}

export async function deletePR(params: DeletePRParams): Promise<{ status: string }> {
  return apiPost<{ status: string }>('/api/prs/delete', params);
}

export interface UpdatePRNotesParams {
  owner: string;
  repo: string;
  number: number;
  notes: string;
}

export async function updatePRNotes(params: UpdatePRNotesParams): Promise<{ status: string; notes: string }> {
  return apiPost<{ status: string; notes: string }>('/api/prs/notes', params);
}

export interface SetPRHiddenParams {
  owner: string;
  repo: string;
  number: number;
  hidden: boolean;
}

export async function setPRHidden(params: SetPRHiddenParams): Promise<{ status: string; hidden: boolean }> {
  return apiPost<{ status: string; hidden: boolean }>('/api/prs/hidden', params);
}

export interface TriggerReviewParams {
  owner: string;
  repo: string;
  number: number;
}

export async function triggerReview(params: TriggerReviewParams): Promise<{ status: string }> {
  return apiPost<{ status: string }>('/api/prs/trigger-review', params);
}
