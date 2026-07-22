import { useCallback } from 'react';
import { createPortal } from 'react-dom';
import type { PR } from '@/types/pr';
import { useTelemetry } from '@/hooks/useTelemetry';
import { useDropdown } from '@/hooks/useDropdown';
import './RowActionsMenu.scss';

// Keep in sync with $panel-width in RowActionsMenu.scss.
const PANEL_WIDTH = 200;

interface RowActionsMenuProps {
  pr: PR;
  onTriggerReview: () => void;
  onToggleHidden: () => void;
  onDelete: () => void;
  /** True while the trigger-review mutation is in flight. */
  reviewPending: boolean;
  /** True while the set-hidden mutation is in flight. */
  hiddenPending: boolean;
  /** True while the delete mutation is in flight. */
  deletePending: boolean;
}

/**
 * Kebab menu for the Actions column. Collapses the per-row "Generate review"
 * and "Delete" controls into a single click-to-open dropdown. The trigger
 * itself surfaces review-in-flight state so status stays visible without
 * opening the menu.
 */
export function RowActionsMenu({ pr, onTriggerReview, onToggleHidden, onDelete, reviewPending, hiddenPending, deletePending }: RowActionsMenuProps) {
  const { track } = useTelemetry();
  const { isOpen, toggle, close, anchorRef, panelRef, position } = useDropdown({
    panelWidth: PANEL_WIDTH,
    align: 'right',
    closeOnOutsideClick: true,
    closeOnEscape: true,
  });

  const reviewInFlight =
    reviewPending || pr.status === 'generating' || pr.status === 'agent_reviewing';
  const hasReview = !!pr.review_url;

  const handleToggle = useCallback(() => {
    if (!isOpen) {
      track('open_row_actions', { pr_owner: pr.owner, pr_repo: pr.repo, pr_number: pr.number });
    }
    toggle();
  }, [isOpen, toggle, track, pr.owner, pr.repo, pr.number]);

  const handleReview = useCallback(() => {
    onTriggerReview();
    close();
  }, [onTriggerReview, close]);

  const handleToggleHidden = useCallback(() => {
    onToggleHidden();
    close();
  }, [onToggleHidden, close]);

  const handleDelete = useCallback(() => {
    onDelete();
    close();
  }, [onDelete, close]);

  const reviewLabel = reviewInFlight
    ? 'Reviewing…'
    : hasReview
      ? '🔄 Regenerate review'
      : '🔄 Generate review';

  return (
    <>
      <button
        ref={anchorRef as React.RefObject<HTMLButtonElement>}
        type="button"
        className="row-actions__trigger"
        aria-label="Actions"
        aria-haspopup="menu"
        aria-expanded={isOpen}
        title="Row actions"
        onClick={handleToggle}
      >
        ⋮
      </button>

      {isOpen && createPortal(
        <div
          ref={panelRef}
          className="row-actions__menu"
          role="menu"
          style={{ top: position.top, left: position.left, width: PANEL_WIDTH }}
        >
          <button
            type="button"
            role="menuitem"
            className="row-actions__item"
            onClick={handleReview}
            disabled={reviewInFlight}
          >
            {reviewLabel}
          </button>
          <button
            type="button"
            role="menuitem"
            className="row-actions__item"
            onClick={handleToggleHidden}
            disabled={hiddenPending}
          >
            {hiddenPending ? 'Updating…' : pr.hidden ? '👁 Unhide' : '🙈 Hide'}
          </button>
          <button
            type="button"
            role="menuitem"
            className="row-actions__item row-actions__item--danger"
            onClick={handleDelete}
            disabled={deletePending}
          >
            {deletePending ? 'Deleting…' : '🗑 Delete'}
          </button>
        </div>,
        document.body
      )}
    </>
  );
}
