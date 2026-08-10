import type { PRStateFilter } from '@/hooks/useUrlFilters';
import type {
  SectionAuthorship,
  SectionConfig,
  SectionFilters,
  SectionsConfig,
} from '@/types/sections';

/**
 * Section layout is a per-browser preference, so it lives in localStorage
 * rather than on the server. The key carries the schema version: a future
 * incompatible shape gets a new key instead of trying to migrate garbage.
 */
export const SECTIONS_STORAGE_KEY = 'prism.review-sections.v1';
export const SECTIONS_CONFIG_VERSION = 1;

const AUTHORSHIPS: SectionAuthorship[] = ['any', 'mine', 'others'];
const PR_STATES: PRStateFilter[] = ['draft', 'ready'];

export function createEmptyFilters(): SectionFilters {
  return {
    authorship: 'any',
    teams: [],
    repos: [],
    authors: [],
    states: [],
    search: '',
  };
}

export function generateSectionId(): string {
  const uuid = globalThis.crypto?.randomUUID?.();
  if (uuid) return uuid;
  return `section-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 10)}`;
}

export function createSection(
  title: string,
  filters: Partial<SectionFilters> = {}
): SectionConfig {
  return {
    id: generateSectionId(),
    title,
    collapsed: false,
    filters: { ...createEmptyFilters(), ...filters },
  };
}

/**
 * The out-of-the-box layout, which reproduces the page's original two
 * sections. Used on first visit and by "Reset to defaults".
 */
export function defaultSectionsConfig(): SectionsConfig {
  return {
    version: SECTIONS_CONFIG_VERSION,
    showUncategorized: true,
    sections: [
      createSection('My PRs', { authorship: 'mine' }),
      createSection('PRs to Review', { authorship: 'others' }),
    ],
  };
}

function sanitizeStringList(value: unknown): string[] {
  if (!Array.isArray(value)) return [];
  const seen = new Set<string>();
  for (const entry of value) {
    if (typeof entry === 'string' && entry !== '') seen.add(entry);
  }
  return Array.from(seen);
}

function sanitizeFilters(value: unknown): SectionFilters {
  const raw = (value ?? {}) as Record<string, unknown>;
  const authorship = raw.authorship as SectionAuthorship;
  const states = sanitizeStringList(raw.states).filter((s): s is PRStateFilter =>
    PR_STATES.includes(s as PRStateFilter)
  );
  return {
    authorship: AUTHORSHIPS.includes(authorship) ? authorship : 'any',
    teams: sanitizeStringList(raw.teams),
    repos: sanitizeStringList(raw.repos),
    authors: sanitizeStringList(raw.authors),
    states,
    search: typeof raw.search === 'string' ? raw.search : '',
  };
}

function sanitizeSection(value: unknown, usedIds: Set<string>): SectionConfig | null {
  if (!value || typeof value !== 'object') return null;
  const raw = value as Record<string, unknown>;
  const id =
    typeof raw.id === 'string' && raw.id !== '' && !usedIds.has(raw.id)
      ? raw.id
      : generateSectionId();
  usedIds.add(id);
  return {
    id,
    title: typeof raw.title === 'string' && raw.title.trim() !== '' ? raw.title : 'Untitled section',
    collapsed: raw.collapsed === true,
    filters: sanitizeFilters(raw.filters),
  };
}

/**
 * Coerces whatever is in storage into a usable config. Anything unrecognizable
 * (hand-edited storage, a payload from an older build) is dropped rather than
 * trusted, and a config with no usable sections falls back to the defaults.
 */
export function sanitizeSectionsConfig(value: unknown): SectionsConfig {
  if (!value || typeof value !== 'object') return defaultSectionsConfig();
  const raw = value as Record<string, unknown>;
  const usedIds = new Set<string>();
  const sections = Array.isArray(raw.sections)
    ? raw.sections
        .map((section) => sanitizeSection(section, usedIds))
        .filter((section): section is SectionConfig => section !== null)
    : [];
  if (sections.length === 0) return defaultSectionsConfig();
  return {
    version: SECTIONS_CONFIG_VERSION,
    sections,
    showUncategorized: raw.showUncategorized !== false,
  };
}

export function loadSectionsConfig(): SectionsConfig {
  try {
    const stored = window.localStorage.getItem(SECTIONS_STORAGE_KEY);
    if (!stored) return defaultSectionsConfig();
    return sanitizeSectionsConfig(JSON.parse(stored));
  } catch {
    // Unparseable JSON, or storage blocked entirely (private mode, disabled
    // cookies). Either way the defaults keep the page usable.
    return defaultSectionsConfig();
  }
}

export function saveSectionsConfig(config: SectionsConfig): void {
  try {
    window.localStorage.setItem(SECTIONS_STORAGE_KEY, JSON.stringify(config));
  } catch {
    // Storage full or unavailable: the in-memory config still applies for
    // this session, it just won't survive a reload.
  }
}

/** Returns a copy of sections with the entry at `from` moved to `to`. */
export function moveSection(
  sections: SectionConfig[],
  from: number,
  to: number
): SectionConfig[] {
  if (from === to) return sections;
  if (from < 0 || from >= sections.length) return sections;
  if (to < 0 || to >= sections.length) return sections;
  const next = [...sections];
  const [moved] = next.splice(from, 1);
  next.splice(to, 0, moved);
  return next;
}

/** Human-readable summary of a section's filters, shown under its title. */
export function describeSectionFilters(filters: SectionFilters): string {
  const parts: string[] = [];
  if (filters.authorship === 'mine') parts.push('authored by me');
  if (filters.authorship === 'others') parts.push('not authored by me');
  if (filters.teams.length > 0) parts.push(`teams: ${filters.teams.join(', ')}`);
  if (filters.repos.length > 0) parts.push(`repos: ${filters.repos.join(', ')}`);
  if (filters.authors.length > 0) parts.push(`authors: ${filters.authors.join(', ')}`);
  if (filters.states.length > 0) parts.push(`state: ${filters.states.join(', ')}`);
  if (filters.search) parts.push(`matching "${filters.search}"`);
  return parts.length > 0 ? parts.join(' · ') : 'all PRs';
}
