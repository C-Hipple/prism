import { act, fireEvent, render, renderHook } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import { computeDropdownPosition, useDropdown } from './useDropdown';
import type { UseDropdownOptions } from './useDropdown';

// A viewport big enough that nothing clamps unless a test sets up for it.
const VIEWPORT = { width: 1024, height: 768 };

// Build an anchor rect from a partial; defaults sit comfortably mid-viewport.
const rect = (partial: Partial<DOMRect>): DOMRect =>
  ({
    top: 100,
    bottom: 120,
    left: 200,
    right: 400,
    width: 200,
    height: 20,
    x: 200,
    y: 100,
    toJSON: () => ({}),
    ...partial,
  }) as DOMRect;

describe('computeDropdownPosition', () => {
  it('right-aligns the panel to the anchor right edge by default', () => {
    const pos = computeDropdownPosition(rect({ right: 500 }), VIEWPORT, { panelWidth: 240 });
    expect(pos.left).toBe(260); // 500 - 240
  });

  it('left-aligns when align is "left"', () => {
    const pos = computeDropdownPosition(rect({ left: 100 }), VIEWPORT, { panelWidth: 240, align: 'left' });
    expect(pos.left).toBe(100);
  });

  it('clamps left to the viewport margin when the anchor hugs the left edge', () => {
    const pos = computeDropdownPosition(rect({ right: 100 }), VIEWPORT, { panelWidth: 240, viewportMargin: 8 });
    expect(pos.left).toBe(8); // 100 - 240 = -140 -> clamped to margin
  });

  it('clamps right so the panel cannot overflow the viewport', () => {
    const pos = computeDropdownPosition(rect({ left: 1000 }), VIEWPORT, { panelWidth: 240, align: 'left', viewportMargin: 8 });
    expect(pos.left).toBe(1024 - 240 - 8); // 776
  });

  it('places the panel below the anchor by default', () => {
    const pos = computeDropdownPosition(rect({ bottom: 200 }), VIEWPORT, { panelWidth: 240, gap: 6 });
    expect(pos.placement).toBe('bottom');
    expect(pos.top).toBe(206); // bottom + gap
  });

  it('flips above when there is no room below and panelHeight is known', () => {
    const pos = computeDropdownPosition(
      rect({ top: 670, bottom: 700 }),
      VIEWPORT,
      { panelWidth: 240, panelHeight: 300, gap: 6, viewportMargin: 8 }
    );
    expect(pos.placement).toBe('top');
    expect(pos.top).toBe(670 - 6 - 300); // anchor.top - gap - panelHeight = 364
  });

  it('does not flip when panelHeight is unknown, even if cramped', () => {
    const pos = computeDropdownPosition(
      rect({ top: 670, bottom: 700 }),
      VIEWPORT,
      { panelWidth: 240, gap: 6 }
    );
    expect(pos.placement).toBe('bottom');
    expect(pos.top).toBe(706);
  });

  it('stays below when neither side has room', () => {
    const pos = computeDropdownPosition(
      rect({ top: 20, bottom: 700 }),
      { width: 1024, height: 720 },
      { panelWidth: 240, panelHeight: 600, gap: 6, viewportMargin: 8 }
    );
    // Below overflows, but above (20 - 6 - 600 < margin) also doesn't fit -> prefer below.
    expect(pos.placement).toBe('bottom');
  });
});

// A tiny harness that wires the hook's refs to real DOM nodes so we can
// exercise outside-click / Escape behavior.
function Harness({ options }: { options?: UseDropdownOptions }) {
  const { isOpen, toggle, anchorRef, panelRef } = useDropdown(options ?? { panelWidth: 200 });
  return (
    <div>
      <button ref={anchorRef as React.RefObject<HTMLButtonElement>} data-testid="trigger" onClick={toggle}>
        open
      </button>
      {isOpen && (
        <div ref={panelRef} data-testid="panel">
          panel
          <button data-testid="inside">inside</button>
        </div>
      )}
      <div data-testid="outside">outside</div>
    </div>
  );
}

describe('useDropdown', () => {
  afterEach(() => cleanupDocument());

  function cleanupDocument() {
    document.body.innerHTML = '';
  }

  it('starts closed and supports open / close / toggle', () => {
    const { result } = renderHook(() => useDropdown({ panelWidth: 200 }));
    expect(result.current.isOpen).toBe(false);
    act(() => result.current.open());
    expect(result.current.isOpen).toBe(true);
    act(() => result.current.close());
    expect(result.current.isOpen).toBe(false);
    act(() => result.current.toggle());
    expect(result.current.isOpen).toBe(true);
  });

  it('closes on Escape when closeOnEscape is set', () => {
    const { getByTestId, queryByTestId } = render(<Harness options={{ panelWidth: 200, closeOnEscape: true }} />);
    fireEvent.click(getByTestId('trigger'));
    expect(queryByTestId('panel')).toBeTruthy();
    fireEvent.keyDown(document, { key: 'Escape' });
    expect(queryByTestId('panel')).toBeNull();
  });

  it('does not close on Escape when closeOnEscape is not set', () => {
    const { getByTestId, queryByTestId } = render(<Harness options={{ panelWidth: 200 }} />);
    fireEvent.click(getByTestId('trigger'));
    fireEvent.keyDown(document, { key: 'Escape' });
    expect(queryByTestId('panel')).toBeTruthy();
  });

  it('closes on outside mousedown but not on inside mousedown', () => {
    const { getByTestId, queryByTestId } = render(<Harness options={{ panelWidth: 200, closeOnOutsideClick: true }} />);
    fireEvent.click(getByTestId('trigger'));
    fireEvent.mouseDown(getByTestId('inside'));
    expect(queryByTestId('panel')).toBeTruthy();
    fireEvent.mouseDown(getByTestId('outside'));
    expect(queryByTestId('panel')).toBeNull();
  });

  it('unmounts cleanly with listeners attached', () => {
    const { getByTestId, unmount } = render(
      <Harness options={{ panelWidth: 200, closeOnEscape: true, closeOnOutsideClick: true }} />
    );
    fireEvent.click(getByTestId('trigger'));
    expect(() => unmount()).not.toThrow();
  });
});
