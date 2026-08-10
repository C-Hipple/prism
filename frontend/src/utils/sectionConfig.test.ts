import { beforeEach, describe, expect, it, vi } from 'vitest';
import {
  SECTIONS_STORAGE_KEY,
  createSection,
  defaultSectionsConfig,
  describeSectionFilters,
  loadSectionsConfig,
  moveSection,
  sanitizeSectionsConfig,
  saveSectionsConfig,
} from './sectionConfig';

describe('sectionConfig', () => {
  beforeEach(() => {
    window.localStorage.clear();
  });

  it('starts from the default two-section layout when nothing is stored', () => {
    const config = loadSectionsConfig();
    expect(config.sections.map((s) => s.title)).toEqual(['My PRs', 'PRs to Review']);
    expect(config.sections.map((s) => s.filters.authorship)).toEqual(['mine', 'others']);
    expect(config.showUncategorized).toBe(true);
  });

  it('round-trips a saved config through localStorage', () => {
    const config = {
      ...defaultSectionsConfig(),
      showUncategorized: false,
      sections: [createSection('Platform', { repos: ['acme/platform'], authors: ['bob'] })],
    };

    saveSectionsConfig(config);
    expect(loadSectionsConfig()).toEqual(config);
  });

  it('falls back to defaults for unparseable or empty stored values', () => {
    window.localStorage.setItem(SECTIONS_STORAGE_KEY, 'not json');
    expect(loadSectionsConfig().sections).toHaveLength(2);

    window.localStorage.setItem(SECTIONS_STORAGE_KEY, JSON.stringify({ sections: [] }));
    expect(loadSectionsConfig().sections).toHaveLength(2);
  });

  it('drops unusable fields from a hand-edited config instead of trusting them', () => {
    const config = sanitizeSectionsConfig({
      version: 99,
      showUncategorized: false,
      sections: [
        {
          id: 'keep-me',
          title: 'Mine',
          collapsed: true,
          filters: {
            authorship: 'bogus',
            teams: ['Platform', 42, 'Platform'],
            repos: 'acme/platform',
            authors: ['bob'],
            states: ['draft', 'exploded'],
            search: 7,
          },
        },
        'garbage',
        { title: 'No id' },
      ],
    });

    expect(config.sections).toHaveLength(2);
    const [first, second] = config.sections;
    expect(first.id).toBe('keep-me');
    expect(first.collapsed).toBe(true);
    expect(first.filters).toEqual({
      authorship: 'any',
      teams: ['Platform'],
      repos: [],
      authors: ['bob'],
      states: ['draft'],
      search: '',
    });
    expect(second.title).toBe('No id');
    expect(second.id).toBeTruthy();
    expect(config.showUncategorized).toBe(false);
  });

  it('gives duplicate ids a fresh identity so React keys stay unique', () => {
    const config = sanitizeSectionsConfig({
      sections: [
        { id: 'dup', title: 'One' },
        { id: 'dup', title: 'Two' },
      ],
    });

    expect(config.sections[0].id).toBe('dup');
    expect(config.sections[1].id).not.toBe('dup');
  });

  it('survives storage being unavailable', () => {
    const getItem = vi
      .spyOn(Storage.prototype, 'getItem')
      .mockImplementation(() => {
        throw new Error('denied');
      });
    const setItem = vi
      .spyOn(Storage.prototype, 'setItem')
      .mockImplementation(() => {
        throw new Error('denied');
      });

    expect(loadSectionsConfig().sections).toHaveLength(2);
    expect(() => saveSectionsConfig(defaultSectionsConfig())).not.toThrow();

    getItem.mockRestore();
    setItem.mockRestore();
  });

  it('moves a section without mutating the original list', () => {
    const sections = [createSection('A'), createSection('B'), createSection('C')];

    expect(moveSection(sections, 2, 0).map((s) => s.title)).toEqual(['C', 'A', 'B']);
    expect(moveSection(sections, 0, 1).map((s) => s.title)).toEqual(['B', 'A', 'C']);
    expect(sections.map((s) => s.title)).toEqual(['A', 'B', 'C']);
  });

  it('ignores out-of-range and no-op moves', () => {
    const sections = [createSection('A'), createSection('B')];
    expect(moveSection(sections, 0, 0)).toBe(sections);
    expect(moveSection(sections, -1, 1)).toBe(sections);
    expect(moveSection(sections, 0, 5)).toBe(sections);
  });

  it('summarizes filters for the section header', () => {
    expect(describeSectionFilters(createSection('x').filters)).toBe('all PRs');
    expect(
      describeSectionFilters(
        createSection('x', {
          authorship: 'others',
          teams: ['Platform'],
          repos: ['acme/api'],
          authors: ['bob'],
          states: ['draft'],
          search: 'billing',
        }).filters
      )
    ).toBe(
      'not authored by me · teams: Platform · repos: acme/api · authors: bob · state: draft · matching "billing"'
    );
  });
});
