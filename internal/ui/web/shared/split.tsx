// Shadcn resizable panels keep hidden views mounted and restore their sizes.
import {Children, useId, useLayoutEffect, useRef, type ReactNode} from 'react';
import type {PanelImperativeHandle} from 'react-resizable-panels';
import {ResizableHandle, ResizablePanel, ResizablePanelGroup} from '../components/ui/resizable';

// Split divides its two children. A vertical split places them side by side
// and a horizontal one stacks them. fixed names the child that keeps its
// pixel size, size is its size in pixels, or the first child's percentage when
// neither is fixed. minStart and minEnd are CSS sizes. collapsed hides one
// child.
interface SplitProps {
  split?: 'vertical' | 'horizontal';
  fixed?: 'start' | 'end' | 'none';
  size: number;
  minStart?: string;
  minEnd?: string;
  collapsed?: 'start' | 'end' | null;
  // User changes report pixels for a fixed child, or the first child's percent.
  onResize?: (size: number) => void;
  children: [ReactNode, ReactNode];
}

export function Split({split = 'vertical', fixed = 'none', size, minStart, minEnd, collapsed = null, onResize, children}: SplitProps) {
  const [start, end] = Children.toArray(children);
  const id = useId();
  const first = useRef<PanelImperativeHandle>(null), last = useRef<PanelImperativeHandle>(null);
  const kept = useRef<number | string>(size);
  const previous = useRef(collapsed);
  useLayoutEffect(() => {
    if (collapsed) (collapsed === 'start' ? first : last).current?.collapse();
    else if (previous.current) {
      const pane = previous.current === 'start' ? first : last;
      pane.current?.resize(fixed === 'none' ? `${size}%` : kept.current);
    }
    previous.current = collapsed;
  }, [collapsed]);
  const startSize = fixed === 'end' ? undefined : fixed === 'start' ? size : `${size}%`;
  return <ResizablePanelGroup orientation={split === 'vertical' ? 'horizontal' : 'vertical'}
    className={`split ${collapsed ? 'split-collapsed' : ''}`} disabled={!!collapsed}
    onLayoutChanged={(layout, meta) => {
      if (!meta.isUserInteraction || collapsed) return;
      if (fixed === 'none') onResize?.(layout[id + '-start']);
      else {
        // Keyboard callbacks precede React's panel styles. Combine the new
        // percentage with the available pixels instead of reading the old size.
        const available = (first.current?.getSize().inPixels ?? 0) + (last.current?.getSize().inPixels ?? 0);
        if (available > 0) onResize?.(available * layout[id + '-' + fixed] / 100);
      }
    }}>
    <ResizablePanel id={id + '-start'} panelRef={first} defaultSize={startSize}
      groupResizeBehavior={fixed === 'start' ? 'preserve-pixel-size' : 'preserve-relative-size'}
      collapsible={fixed === 'start'} minSize={minStart}
      onResize={value => { if (fixed === 'start' && !collapsed && value.inPixels > 0) kept.current = value.inPixels; }}>
      <div className="split-pane h-full min-h-0 min-w-0 overflow-hidden" data-side="start" inert={collapsed === 'start'}>{start}</div>
    </ResizablePanel>
    <ResizableHandle hidden={!!collapsed} />
    <ResizablePanel id={id + '-end'} panelRef={last} defaultSize={fixed === 'end' ? size : undefined}
      groupResizeBehavior={fixed === 'end' ? 'preserve-pixel-size' : 'preserve-relative-size'}
      collapsible={fixed === 'end'} minSize={minEnd}
      onResize={value => { if (fixed === 'end' && !collapsed && value.inPixels > 0) kept.current = value.inPixels; }}>
      <div className="split-pane h-full min-h-0 min-w-0 overflow-hidden" data-side="end" inert={collapsed === 'end'}>{end}</div>
    </ResizablePanel>
  </ResizablePanelGroup>;
}
