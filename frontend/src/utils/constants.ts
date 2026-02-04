// Polling intervals
// With WebSockets, we can disable periodic background polling for data
// 0 means disabled for React Query refetchInterval
export const PR_POLL_INTERVAL = 0; 
export const STATUS_POLL_INTERVAL = 10000; // Keep status polling for heartbeat/uptime
export const PRIORITY_POLL_INTERVAL = 0;
export const HEALTH_POLL_INTERVAL = 0;

// React Query stale time
export const PR_STALE_TIME = 15000; 
export const STATUS_STALE_TIME = 5000;
export const PRIORITY_STALE_TIME = 30000;
