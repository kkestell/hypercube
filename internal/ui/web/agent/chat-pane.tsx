// Chat is a permanent session pane, not a draggable or closeable editor tab.
import {useRef} from 'react';
import {LayoutToggles} from '../app/layout-toggles.tsx';
import {topRow, topRowStart} from '../app/title-bar.ts';
import type {Session} from '../sessions/types.ts';
import {Icon} from '../shared/icon.tsx';
import {tabBackground} from '../shared/tab-background.ts';
import {usePaneFocus} from '../shared/use-pane-focus.ts';
import {useStore} from '../store.ts';
import {AgentPane} from './agent-pane.tsx';
import {WorkingContext} from './working-context.tsx';

export function ChatPane({session}: {session: Session}) {
  const [state] = useStore();
  const pane = useRef<HTMLDivElement>(null);
  const focused = usePaneFocus(pane);
  return <div ref={pane} className="chat-pane flex h-full min-h-0 min-w-0 flex-col" aria-label="Chat">
    <div className={`flex min-w-0 shrink-0 items-center gap-2 border-b px-2 ${tabBackground} ${topRow} ${!state.workbench.sidebar ? topRowStart : ''}`}>
      <Icon name="comment-discussion" />
      <span className="min-w-0 flex-1 truncate text-sm font-medium" title={session.title}>{session.title}</span>
      <LayoutToggles />
    </div>
    <WorkingContext />
    <AgentPane session={session} active={focused} />
  </div>;
}
