import { act, renderHook } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { useUrlFilters } from './useUrlFilters';

function currentUrl(): string {
  return `${window.location.pathname}${window.location.search}`;
}

describe('useUrlFilters', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    window.history.replaceState(null, '', '/');
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.restoreAllMocks();
  });

  it('starts empty when the URL has no filter params', () => {
    const { result } = renderHook(() => useUrlFilters());
    expect(result.current.searchTerm).toBe('');
    expect(result.current.selectedTeams).toEqual([]);
    expect(result.current.selectedRepos).toEqual([]);
    expect(result.current.selectedStates).toEqual([]);
  });

  it('initializes from URL params', () => {
    window.history.replaceState(null, '', '/?q=fix&team=core&team=infra&repo=acme%2Fexample&state=draft');
    const { result } = renderHook(() => useUrlFilters());
    expect(result.current.searchTerm).toBe('fix');
    expect(result.current.selectedTeams).toEqual(['core', 'infra']);
    expect(result.current.selectedRepos).toEqual(['acme/example']);
    expect(result.current.selectedStates).toEqual(['draft']);
  });

  it('ignores unknown state values in the URL', () => {
    window.history.replaceState(null, '', '/?state=draft&state=bogus&state=ready');
    const { result } = renderHook(() => useUrlFilters());
    expect(result.current.selectedStates).toEqual(['draft', 'ready']);
  });

  it('pushes state changes to the URL immediately', () => {
    const { result } = renderHook(() => useUrlFilters());
    act(() => result.current.setSelectedStates(['ready']));
    expect(currentUrl()).toBe('/?state=ready');
    act(() => result.current.setSelectedStates([]));
    expect(currentUrl()).toBe('/');
  });

  it('pushes team and repo changes to the URL immediately', () => {
    const { result } = renderHook(() => useUrlFilters());

    act(() => result.current.setSelectedTeams(['core']));
    expect(currentUrl()).toBe('/?team=core');

    act(() => result.current.setSelectedRepos(['acme/example']));
    expect(currentUrl()).toBe('/?team=core&repo=acme%2Fexample');
  });

  it('debounces search term pushes into a single history entry', () => {
    const { result } = renderHook(() => useUrlFilters());
    const pushSpy = vi.spyOn(window.history, 'pushState');

    act(() => result.current.setSearchTerm('f'));
    act(() => result.current.setSearchTerm('fi'));
    act(() => result.current.setSearchTerm('fix'));

    expect(result.current.searchTerm).toBe('fix'); // state updates immediately
    expect(pushSpy).not.toHaveBeenCalled(); // URL waits for the debounce

    act(() => {
      vi.runAllTimers();
    });
    expect(pushSpy).toHaveBeenCalledTimes(1);
    expect(currentUrl()).toBe('/?q=fix');
  });

  it('restores filter state on popstate (back/forward)', () => {
    const { result } = renderHook(() => useUrlFilters());

    act(() => result.current.setSelectedTeams(['core']));
    act(() => result.current.setSelectedTeams(['core', 'infra']));
    expect(currentUrl()).toBe('/?team=core&team=infra');

    act(() => {
      window.history.back();
    });
    // jsdom fires popstate asynchronously; simulate it landing on the prior URL
    act(() => {
      window.history.replaceState(null, '', '/?team=core');
      window.dispatchEvent(new PopStateEvent('popstate'));
    });
    expect(result.current.selectedTeams).toEqual(['core']);
  });

  it('cancels a pending search push when popstate fires', () => {
    const { result } = renderHook(() => useUrlFilters());
    const pushSpy = vi.spyOn(window.history, 'pushState');

    act(() => result.current.setSearchTerm('fix'));
    act(() => {
      window.history.replaceState(null, '', '/?team=core');
      window.dispatchEvent(new PopStateEvent('popstate'));
    });
    act(() => {
      vi.runAllTimers();
    });

    expect(pushSpy).not.toHaveBeenCalled();
    expect(result.current.searchTerm).toBe('');
    expect(result.current.selectedTeams).toEqual(['core']);
  });

  it('removes params when filters are cleared', () => {
    window.history.replaceState(null, '', '/?q=fix&team=core');
    const { result } = renderHook(() => useUrlFilters());

    act(() => result.current.setSelectedTeams([]));
    act(() => result.current.setSearchTerm(''));
    act(() => {
      vi.runAllTimers();
    });
    expect(currentUrl()).toBe('/');
  });

  it('preserves unrelated query params', () => {
    window.history.replaceState(null, '', '/?debug=1');
    const { result } = renderHook(() => useUrlFilters());

    act(() => result.current.setSelectedTeams(['core']));
    expect(currentUrl()).toBe('/?debug=1&team=core');
  });

  it('does not push a history entry when the URL would not change', () => {
    window.history.replaceState(null, '', '/?team=core');
    const { result } = renderHook(() => useUrlFilters());
    const pushSpy = vi.spyOn(window.history, 'pushState');

    act(() => result.current.setSelectedTeams(['core']));
    expect(pushSpy).not.toHaveBeenCalled();
  });
});
