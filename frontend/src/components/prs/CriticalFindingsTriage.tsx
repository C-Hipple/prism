import { useCallback, useEffect, useMemo, useState } from 'react';
import type { PR } from '@/types/pr';
import { useTelemetry } from '@/hooks/useTelemetry';
import {
  fetchCriticalFindings,
  fetchFindingOutcomes,
  recordFindingOutcome,
  type FindingOutcome,
  type FindingOutcomeKind,
  type ReviewFinding,
} from '@/api/findingOutcomes';
import './CriticalFindingsTriage.scss';

interface CriticalFindingsTriageProps {
  pr: PR;
}

const COMMENT_EXCERPT_LEN = 140;

function excerpt(comment: string): string {
  const oneLine = comment.replace(/\s+/g, ' ').trim();
  return oneLine.length > COMMENT_EXCERPT_LEN ? `${oneLine.slice(0, COMMENT_EXCERPT_LEN)}…` : oneLine;
}

// Server fingerprints are "<file>:<line/10>:<hash>"; the hash covers the
// comment text, which the client doesn't re-hash. Within ONE review two
// findings can't share a file + line bucket (merge dedup collapses them), so
// the prefix pairs a sidecar finding with its recorded outcome — but only
// when scoped to that review's SHA: outcomes are listed across all reviewed
// SHAs, and an old review's dismissal in the same bucket must not render a
// different, newer finding as already triaged (hiding its actions forever).
function outcomeFor(
  outcomes: FindingOutcome[],
  f: ReviewFinding,
  reviewedSha: string
): FindingOutcome | undefined {
  const sha = reviewedSha.toLowerCase();
  const prefix = `${f.file}:${Math.floor(f.line / 10)}:`;
  return outcomes.find(
    o => o.reviewed_sha.toLowerCase() === sha && o.fingerprint.startsWith(prefix)
  );
}

/**
 * Minimal dismiss-with-reason / acknowledge affordance for a review's
 * CRITICAL findings (label harvesting, not a review UI). Rendered inside the
 * ReviewLinkMenu panel; probes the feature-gated outcomes endpoint on mount
 * and renders nothing when the backend has the feature disabled (404).
 */
export function CriticalFindingsTriage({ pr }: CriticalFindingsTriageProps) {
  const { track } = useTelemetry();
  // null = probing; false = feature disabled/unreachable; true = enabled.
  const [enabled, setEnabled] = useState<boolean | null>(null);
  const [outcomes, setOutcomes] = useState<FindingOutcome[]>([]);
  const [expanded, setExpanded] = useState(false);
  // null = not fetched yet (fetched lazily on first expand).
  const [findings, setFindings] = useState<ReviewFinding[] | null>(null);
  const [reviewSha, setReviewSha] = useState('');
  const [dismissingIdx, setDismissingIdx] = useState<number | null>(null);
  const [reason, setReason] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const trackOpts = useMemo(
    () => ({ pr_owner: pr.owner, pr_repo: pr.repo, pr_number: pr.number }),
    [pr.owner, pr.repo, pr.number]
  );

  // Feature probe + existing outcomes. A 404 (feature off) or any failure
  // hides the affordance entirely — the menu renders exactly as before.
  useEffect(() => {
    let cancelled = false;
    fetchFindingOutcomes(pr.owner, pr.repo, pr.number)
      .then(existing => {
        if (cancelled) return;
        setOutcomes(existing);
        setEnabled(true);
      })
      .catch(() => {
        if (!cancelled) setEnabled(false);
      });
    return () => {
      cancelled = true;
    };
  }, [pr.owner, pr.repo, pr.number]);

  const toggleExpanded = useCallback(() => {
    setExpanded(prev => !prev);
    if (findings === null) {
      fetchCriticalFindings(pr.owner, pr.repo, pr.number)
        .then(({ sha, findings: criticals }) => {
          setReviewSha(sha);
          setFindings(criticals);
        })
        .catch(() => setFindings([]));
    }
  }, [findings, pr.owner, pr.repo, pr.number]);

  const submit = useCallback(
    async (f: ReviewFinding, outcome: FindingOutcomeKind, submitReason: string) => {
      setBusy(true);
      setError(null);
      try {
        await recordFindingOutcome({
          owner: pr.owner,
          repo: pr.repo,
          number: pr.number,
          sha: reviewSha || pr.commit_sha,
          file: f.file,
          line: f.line,
          comment: f.comment,
          severity: f.severity,
          provenance: f.provenance,
          outcome,
          reason: submitReason,
        });
        track('finding_outcome_recorded', trackOpts);
        const fresh = await fetchFindingOutcomes(pr.owner, pr.repo, pr.number);
        setOutcomes(fresh);
        setDismissingIdx(null);
        setReason('');
      } catch {
        setError('Failed to record — try again');
      }
      setBusy(false);
    },
    [pr.owner, pr.repo, pr.number, pr.commit_sha, reviewSha, track, trackOpts]
  );

  if (enabled !== true) return null;

  return (
    <div className="finding-triage">
      <button
        type="button"
        className="review-menu__item review-menu__item--button"
        role="menuitem"
        aria-expanded={expanded}
        onClick={toggleExpanded}
      >
        <span className="review-menu__icon">⚑</span>
        Review findings… ({pr.critical_count} critical)
      </button>

      {expanded && (
        <div className="finding-triage__list">
          {findings === null && <div className="finding-triage__meta">Loading findings…</div>}
          {findings !== null && findings.length === 0 && (
            <div className="finding-triage__meta">
              No structured critical findings — regenerate the review to triage.
            </div>
          )}
          {findings?.map((f, i) => {
            // Outcomes are recorded against reviewSha (pr.commit_sha when the
            // sidecar carried none) — pair with the same key.
            const existing = outcomeFor(outcomes, f, reviewSha || pr.commit_sha);
            return (
              <div className="finding-triage__finding" key={`${f.file}:${f.line}:${i}`}>
                <div className="finding-triage__loc">
                  {f.file}:{f.line}
                </div>
                <div className="finding-triage__comment">{excerpt(f.comment)}</div>
                {existing ? (
                  <div className={`finding-triage__state finding-triage__state--${existing.outcome}`}>
                    {existing.outcome === 'dismissed'
                      ? `Dismissed by ${existing.decided_by}: ${existing.reason ?? ''}`
                      : `${existing.outcome} by ${existing.decided_by}`}
                  </div>
                ) : dismissingIdx === i ? (
                  <div className="finding-triage__reason-row">
                    <input
                      type="text"
                      className="finding-triage__reason-input"
                      value={reason}
                      maxLength={2000}
                      placeholder="Why is this not a bug? (required)"
                      aria-label={`Dismiss reason for ${f.file}:${f.line}`}
                      onChange={e => setReason(e.target.value)}
                    />
                    <button
                      type="button"
                      className="finding-triage__btn finding-triage__btn--danger"
                      disabled={busy || reason.trim() === ''}
                      onClick={() => submit(f, 'dismissed', reason.trim())}
                    >
                      Dismiss
                    </button>
                    <button
                      type="button"
                      className="finding-triage__btn"
                      disabled={busy}
                      onClick={() => {
                        setDismissingIdx(null);
                        setReason('');
                      }}
                    >
                      Cancel
                    </button>
                  </div>
                ) : (
                  <div className="finding-triage__actions">
                    <button
                      type="button"
                      className="finding-triage__btn"
                      disabled={busy}
                      onClick={() => submit(f, 'acknowledged', '')}
                    >
                      Acknowledge
                    </button>
                    <button
                      type="button"
                      className="finding-triage__btn finding-triage__btn--danger"
                      disabled={busy}
                      onClick={() => {
                        setDismissingIdx(i);
                        setReason('');
                      }}
                    >
                      Dismiss…
                    </button>
                  </div>
                )}
              </div>
            );
          })}
          {error && <div className="finding-triage__error">{error}</div>}
        </div>
      )}
    </div>
  );
}
