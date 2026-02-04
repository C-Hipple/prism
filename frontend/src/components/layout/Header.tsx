import { useState } from 'react';

export function Header() {
  const [isPolling, setIsPolling] = useState(false);

  const handleTriggerPoll = async () => {
    setIsPolling(true);
    try {
      await fetch('/api/poll/trigger', { method: 'POST' });
    } catch (error) {
      console.error('Failed to trigger poll:', error);
    } finally {
      // Keep spinner for a bit to indicate activity
      setTimeout(() => setIsPolling(false), 2000);
    }
  };

  return (
    <header className="app-header">
      <h1>PR Review Dashboard</h1>
      <button
        onClick={handleTriggerPoll}
        disabled={isPolling}
        style={{
          padding: '8px 16px',
          fontSize: '14px',
          borderRadius: '6px',
          border: '1px solid #30363d',
          backgroundColor: '#21262d',
          color: '#c9d1d9',
          cursor: isPolling ? 'not-allowed' : 'pointer',
          display: 'flex',
          alignItems: 'center',
          gap: '8px',
        }}
      >
        {isPolling ? '🔄 Polling...' : '🔄 Refresh PRs'}
      </button>
    </header>
  );
}
