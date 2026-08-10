import { useId } from 'react';
import type { PRStateFilter } from '@/hooks/useUrlFilters';
import type { SectionAuthorship, SectionConfig, SectionFilters } from '@/types/sections';
import { createEmptyFilters } from '@/utils/sectionConfig';

const AUTHORSHIP_OPTIONS: { value: SectionAuthorship; label: string }[] = [
  { value: 'any', label: 'Anyone' },
  { value: 'mine', label: 'Me' },
  { value: 'others', label: 'Everyone but me' },
];

const STATE_OPTIONS: { value: PRStateFilter; label: string }[] = [
  { value: 'ready', label: 'Ready for review' },
  { value: 'draft', label: 'Draft' },
];

interface SectionEditorProps {
  section: SectionConfig;
  availableTeams: string[];
  availableRepos: string[];
  availableAuthors: string[];
  onTitleChange: (title: string) => void;
  onFiltersChange: (filters: SectionFilters) => void;
  onDone: () => void;
}

function toggle<T>(values: T[], value: T): T[] {
  return values.includes(value) ? values.filter((v) => v !== value) : [...values, value];
}

interface ChipListProps<T extends string> {
  label: string;
  options: { value: T; label: string }[];
  selected: T[];
  emptyHint: string;
  onToggle: (value: T) => void;
}

function ChipList<T extends string>({
  label,
  options,
  selected,
  emptyHint,
  onToggle,
}: ChipListProps<T>) {
  const labelId = useId();
  return (
    <div className="section-editor__row">
      <span className="section-editor__label" id={labelId}>
        {label}
      </span>
      {options.length === 0 ? (
        <span className="section-editor__hint">{emptyHint}</span>
      ) : (
        <div className="section-editor__chips" role="group" aria-labelledby={labelId}>
          {options.map(({ value, label: optionLabel }) => {
            const active = selected.includes(value);
            return (
              <button
                key={value}
                type="button"
                className={`section-chip ${active ? 'section-chip--active' : ''}`}
                aria-pressed={active}
                onClick={() => onToggle(value)}
              >
                {optionLabel}
              </button>
            );
          })}
        </div>
      )}
    </div>
  );
}

/**
 * Inline filter editor for one section. Every control writes through
 * immediately — there is no save step, because the caller persists the config
 * on each change.
 */
export function SectionEditor({
  section,
  availableTeams,
  availableRepos,
  availableAuthors,
  onTitleChange,
  onFiltersChange,
  onDone,
}: SectionEditorProps) {
  const { filters } = section;
  const authorshipLabelId = useId();
  const patch = (changes: Partial<SectionFilters>) =>
    onFiltersChange({ ...filters, ...changes });

  // Values saved earlier may no longer be present in the current PR set (a repo
  // with nothing open, say). Keep offering them so they stay removable.
  const withSelected = (options: string[], selected: string[]) =>
    Array.from(new Set([...options, ...selected]))
      .sort()
      .map((value) => ({ value, label: value }));

  return (
    <div className="section-editor">
      <div className="section-editor__row">
        <label className="section-editor__label" htmlFor={`section-title-${section.id}`}>
          Name
        </label>
        <input
          id={`section-title-${section.id}`}
          className="section-editor__text-input"
          type="text"
          value={section.title}
          onChange={(e) => onTitleChange(e.target.value)}
        />
      </div>

      <div className="section-editor__row">
        <span className="section-editor__label" id={authorshipLabelId}>
          Author is
        </span>
        <div className="section-editor__chips" role="group" aria-labelledby={authorshipLabelId}>
          {AUTHORSHIP_OPTIONS.map(({ value, label }) => (
            <button
              key={value}
              type="button"
              className={`section-chip ${filters.authorship === value ? 'section-chip--active' : ''}`}
              aria-pressed={filters.authorship === value}
              onClick={() => patch({ authorship: value })}
            >
              {label}
            </button>
          ))}
        </div>
      </div>

      <ChipList
        label="Teams"
        options={withSelected(availableTeams, filters.teams)}
        selected={filters.teams}
        emptyHint="No teams on the current PRs"
        onToggle={(team) => patch({ teams: toggle(filters.teams, team) })}
      />

      <ChipList
        label="Repos"
        options={withSelected(availableRepos, filters.repos)}
        selected={filters.repos}
        emptyHint="No repos on the current PRs"
        onToggle={(repo) => patch({ repos: toggle(filters.repos, repo) })}
      />

      <ChipList
        label="Authors"
        options={withSelected(availableAuthors, filters.authors)}
        selected={filters.authors}
        emptyHint="No authors on the current PRs"
        onToggle={(author) => patch({ authors: toggle(filters.authors, author) })}
      />

      <ChipList
        label="State"
        options={STATE_OPTIONS}
        selected={filters.states}
        emptyHint=""
        onToggle={(state) => patch({ states: toggle(filters.states, state) })}
      />

      <div className="section-editor__row">
        <label className="section-editor__label" htmlFor={`section-search-${section.id}`}>
          Text
        </label>
        <input
          id={`section-search-${section.id}`}
          className="section-editor__text-input"
          type="text"
          placeholder="Title, repo, author or number contains..."
          value={filters.search}
          onChange={(e) => patch({ search: e.target.value })}
        />
      </div>

      <div className="section-editor__footer">
        <button
          type="button"
          className="section-editor__clear"
          onClick={() => onFiltersChange(createEmptyFilters())}
        >
          Clear filters
        </button>
        <button type="button" className="section-editor__done" onClick={onDone}>
          Done
        </button>
      </div>
    </div>
  );
}
