import type { PR } from '@/types/pr';
import type { SectionConfig, SectionFilters } from '@/types/sections';
import { describeSectionFilters } from '@/utils/sectionConfig';
import { PRTable } from './PRTable';
import { SectionEditor } from './SectionEditor';

interface PRSectionProps {
  section: SectionConfig;
  prs: PR[];
  index: number;
  sectionCount: number;
  customizing: boolean;
  editing: boolean;
  availableTeams: string[];
  availableRepos: string[];
  availableAuthors: string[];
  onToggleCollapsed: () => void;
  onToggleEditing: () => void;
  onTitleChange: (title: string) => void;
  onFiltersChange: (filters: SectionFilters) => void;
  onDuplicate: () => void;
  onRemove: () => void;
  onMove: (from: number, to: number) => void;
  onDragStart: (index: number) => void;
  onDragEnd: () => void;
  onDropOn: (index: number) => void;
  dragging: boolean;
}

export function PRSection({
  section,
  prs,
  index,
  sectionCount,
  customizing,
  editing,
  availableTeams,
  availableRepos,
  availableAuthors,
  onToggleCollapsed,
  onToggleEditing,
  onTitleChange,
  onFiltersChange,
  onDuplicate,
  onRemove,
  onMove,
  onDragStart,
  onDragEnd,
  onDropOn,
  dragging,
}: PRSectionProps) {
  const expanded = !section.collapsed;
  // Dragging is suspended while the editor is open so text selection in its
  // inputs still works.
  const reorderable = customizing && !editing;

  return (
    <section
      className={`pr-section ${dragging ? 'pr-section--dragging' : ''}`}
      data-testid={`pr-section-${section.id}`}
      draggable={reorderable}
      onDragStart={(e) => {
        if (!reorderable) return;
        e.dataTransfer.effectAllowed = 'move';
        // Firefox refuses to start a drag without drag data.
        e.dataTransfer.setData('text/plain', String(index));
        onDragStart(index);
      }}
      onDragEnd={onDragEnd}
      onDragOver={(e) => {
        if (!customizing) return;
        e.preventDefault();
        e.dataTransfer.dropEffect = 'move';
      }}
      onDrop={(e) => {
        if (!customizing) return;
        e.preventDefault();
        onDropOn(index);
      }}
    >
      <div className="section-header">
        <h2 className="pr-section__heading">
          <button
            type="button"
            className="pr-section__toggle"
            aria-expanded={expanded}
            aria-label={`${expanded ? 'Collapse' : 'Expand'} ${section.title}`}
            onClick={onToggleCollapsed}
          >
            <span className="pr-section__chevron" aria-hidden="true">
              {expanded ? '▾' : '▸'}
            </span>
          </button>
          <span className="pr-section__title" data-testid="section-title">
            {section.title} ({prs.length})
          </span>
        </h2>

        {customizing && (
          <div className="section-header__actions">
            <span className="pr-section__grip" aria-hidden="true" title="Drag to reorder">
              ⠿
            </span>
            <button
              type="button"
              className="column-toggle-btn"
              aria-label={`Move ${section.title} up`}
              disabled={index === 0}
              onClick={() => onMove(index, index - 1)}
            >
              ↑
            </button>
            <button
              type="button"
              className="column-toggle-btn"
              aria-label={`Move ${section.title} down`}
              disabled={index === sectionCount - 1}
              onClick={() => onMove(index, index + 1)}
            >
              ↓
            </button>
            <button
              type="button"
              className="column-toggle-btn"
              aria-expanded={editing}
              onClick={onToggleEditing}
            >
              {editing ? 'Close' : 'Edit'}
            </button>
            <button
              type="button"
              className="column-toggle-btn"
              aria-label={`Duplicate ${section.title}`}
              onClick={onDuplicate}
            >
              Duplicate
            </button>
            <button
              type="button"
              className="column-toggle-btn"
              aria-label={`Delete ${section.title}`}
              onClick={onRemove}
            >
              Delete
            </button>
          </div>
        )}
      </div>

      {customizing && !editing && (
        <p className="pr-section__summary">{describeSectionFilters(section.filters)}</p>
      )}

      {editing && (
        <SectionEditor
          section={section}
          availableTeams={availableTeams}
          availableRepos={availableRepos}
          availableAuthors={availableAuthors}
          onTitleChange={onTitleChange}
          onFiltersChange={onFiltersChange}
          onDone={onToggleEditing}
        />
      )}

      {expanded &&
        (prs.length === 0 ? (
          <p className="review-prs__empty-state">No PRs match this section</p>
        ) : (
          <PRTable prs={prs} showViaTeams={section.filters.authorship !== 'mine'} />
        ))}
    </section>
  );
}
