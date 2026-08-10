import { useCallback, useRef, useState } from 'react';
import type { SectionConfig, SectionFilters, SectionsConfig } from '@/types/sections';
import {
  createSection,
  defaultSectionsConfig,
  loadSectionsConfig,
  moveSection as moveSectionInList,
  saveSectionsConfig,
} from '@/utils/sectionConfig';

/**
 * The user's section layout, backed by localStorage. Every mutation writes the
 * whole config straight back to storage, so the layout is never left in an
 * unsaved state.
 */
export function useSectionsConfig() {
  const [config, setConfig] = useState<SectionsConfig>(loadSectionsConfig);
  const configRef = useRef(config);

  const commit = useCallback((next: SectionsConfig) => {
    configRef.current = next;
    saveSectionsConfig(next);
    setConfig(next);
  }, []);

  const updateSections = useCallback(
    (updater: (sections: SectionConfig[]) => SectionConfig[]) => {
      commit({ ...configRef.current, sections: updater(configRef.current.sections) });
    },
    [commit]
  );

  const addSection = useCallback(
    (title = 'New section') => {
      const section = createSection(title);
      updateSections((sections) => [...sections, section]);
      return section;
    },
    [updateSections]
  );

  const removeSection = useCallback(
    (id: string) => {
      updateSections((sections) => sections.filter((section) => section.id !== id));
    },
    [updateSections]
  );

  const renameSection = useCallback(
    (id: string, title: string) => {
      updateSections((sections) =>
        sections.map((section) => (section.id === id ? { ...section, title } : section))
      );
    },
    [updateSections]
  );

  const setSectionFilters = useCallback(
    (id: string, filters: SectionFilters) => {
      updateSections((sections) =>
        sections.map((section) => (section.id === id ? { ...section, filters } : section))
      );
    },
    [updateSections]
  );

  const setSectionCollapsed = useCallback(
    (id: string, collapsed: boolean) => {
      updateSections((sections) =>
        sections.map((section) => (section.id === id ? { ...section, collapsed } : section))
      );
    },
    [updateSections]
  );

  const duplicateSection = useCallback(
    (id: string) => {
      updateSections((sections) => {
        const index = sections.findIndex((section) => section.id === id);
        if (index === -1) return sections;
        const source = sections[index];
        const copy = createSection(`${source.title} (copy)`, source.filters);
        return [...sections.slice(0, index + 1), copy, ...sections.slice(index + 1)];
      });
    },
    [updateSections]
  );

  const moveSection = useCallback(
    (from: number, to: number) => {
      updateSections((sections) => moveSectionInList(sections, from, to));
    },
    [updateSections]
  );

  const setShowUncategorized = useCallback(
    (showUncategorized: boolean) => {
      commit({ ...configRef.current, showUncategorized });
    },
    [commit]
  );

  const resetSections = useCallback(() => {
    commit(defaultSectionsConfig());
  }, [commit]);

  return {
    config,
    sections: config.sections,
    showUncategorized: config.showUncategorized,
    addSection,
    removeSection,
    renameSection,
    setSectionFilters,
    setSectionCollapsed,
    duplicateSection,
    moveSection,
    setShowUncategorized,
    resetSections,
  };
}
