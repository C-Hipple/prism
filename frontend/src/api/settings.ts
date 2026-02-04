import { apiGet, apiPost } from './client';

export interface Settings {
  auto_review_requested_prs: boolean;
}

export async function fetchSettings(): Promise<Settings> {
  return apiGet<Settings>('/api/settings');
}

export async function updateSettings(settings: Partial<Settings>): Promise<Settings> {
  return apiPost<Settings>('/api/settings', settings);
}
