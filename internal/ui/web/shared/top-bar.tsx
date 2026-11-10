// A tab bar with its trailing actions. A bar in the window's top row is the
// window's title bar: at least its height, dragging the window from its empty
// space. A bar at the window's left edge clears the window's buttons, always
// for the sidebar and, for a bar beside it, while the sidebar is hidden.
import type {ReactNode} from 'react';
import {topRow, topRowStart} from '../app/title-bar.ts';
import {useStore} from '../store.ts';

interface TopBarProps {
  top?: boolean;
  leftEdge?: 'always' | 'withoutSidebar' | 'never';
  className?: string;
  actions?: ReactNode;
  children: ReactNode;
}

export function TopBar({top = true, leftEdge = 'never', className = '', actions, children}: TopBarProps) {
  const [state] = useStore();
  const clears = top && (leftEdge === 'always' || (leftEdge === 'withoutSidebar' && !state.workbench.sidebar));
  return <div className={`flex min-w-0 shrink-0 items-center justify-between gap-1 px-2 ${top ? topRow : 'py-1'} ${clears ? topRowStart : ''} ${className}`}>
    {children}
    {actions && <div className="flex shrink-0 items-center">{actions}</div>}
  </div>;
}
