// Chat is a permanent session pane, not a draggable or closeable editor tab.
import {useRef} from 'react';
import {MainTabBar} from '../editor/main-tab-bar.tsx';
import type {Session} from '../sessions/types.ts';
import {usePaneFocus} from '../shared/use-pane-focus.ts';
import {AgentPane} from './agent-pane.tsx';
import {WorkingContext} from './working-context.tsx';

export function ChatPane({session}: {session: Session}) {
  const pane = useRef<HTMLDivElement>(null);
  const focused = usePaneFocus(pane);
  return <div ref={pane} className="flex h-full min-h-0 min-w-0 flex-col" aria-label="Chat">
    <MainTabBar icon="comment-discussion" title={session.title} />
    <WorkingContext />
    <AgentPane session={session} active={focused} />
  </div>;
}
