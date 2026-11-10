// A session's chat: its working directory and its agent. ChatView fills the
// main tab; ChatPane draws it under its own tab bar for the unsent session,
// which has no tabs. While chat shows and the page is visible, the session's
// unseen reply is seen.
import {useEffect, useRef} from 'react';
import {markSeen} from '../sessions/session-actions.ts';
import {useStore} from '../store.ts';
import {MainTabBar} from '../editor/main-tab-bar.tsx';
import type {Session} from '../sessions/types.ts';
import {usePaneFocus} from '../shared/use-pane-focus.ts';
import {AgentPane} from './agent-pane.tsx';
import {WorkingContext} from './working-context.tsx';

export function ChatView({session, active}: {session: Session; active: boolean}) {
  const [state, store] = useStore();
  useEffect(() => {
    if (session.unseen && state.visible) markSeen(store, session.id);
  }, [session.id, session.unseen, state.visible]);
  return <div className="flex min-h-0 min-w-0 flex-1 flex-col" aria-label="Chat">
    <WorkingContext />
    <AgentPane session={session} active={active} />
  </div>;
}

export function ChatPane({session}: {session: Session}) {
  const pane = useRef<HTMLDivElement>(null);
  const focused = usePaneFocus(pane);
  return <div ref={pane} className="flex h-full min-h-0 min-w-0 flex-col">
    <MainTabBar icon="comment-discussion" title={session.title} />
    <ChatView session={session} active={focused} />
  </div>;
}
