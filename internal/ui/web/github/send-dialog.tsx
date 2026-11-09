// The session picker for a pull request action: a new session, or an existing
// one, receives the action's instruction.
import {CommandGroup, CommandInput, CommandItem, CommandList} from '../components/ui/command';
import type {Session, Workspace} from '../sessions/types.ts';
import {statusOf} from '../sessions/session-status.tsx';
import {CommandDialog} from '../shared/command-dialog.tsx';
import {useStore} from '../store.ts';
import {actionLabels, instruction, sendInstruction, startInstruction, type PullRequestAction} from './instructions.ts';
import type {PullRequest} from './types.ts';

// candidates are the workspace's sessions that have not ended, with those on
// the pull request first.
function candidates(workspace: Workspace, pr: PullRequest): Session[] {
  const live = workspace.sessions.flatMap(session => [session, ...session.children]).filter(session => !session.ended);
  const matching = (session: Session) => session.pullRequest?.number === pr.number;
  return [...live.filter(matching), ...live.filter(session => !matching(session))];
}

interface SendDialogProps {
  workspace: Workspace;
  pr: PullRequest;
  action: PullRequestAction;
  onClose: () => void;
}

export function SendDialog({workspace, pr, action, onClose}: SendDialogProps) {
  const [state, store] = useStore();
  const text = instruction(action, pr);
  const title = `${actionLabels[action]} for #${pr.number}`;
  const choose = (send: () => unknown) => { onClose(); send(); };
  return <CommandDialog open onOpenChange={open => { if (!open) onClose(); }} title={title} description="Choose the session that receives the instruction">
    <CommandInput placeholder="Choose a session..." />
    <CommandList>
      <CommandGroup>
        <CommandItem value="New Session" onSelect={() => choose(() => startInstruction(store, workspace.id, text))}>New Session</CommandItem>
        {candidates(workspace, pr).map(session => <CommandItem key={session.id} value={`${session.id} ${session.title}`} onSelect={() => choose(() => sendInstruction(store, session.id, text))}>
          {statusOf(session, false, session.attention || state.requests.some(request => request.session === session.id), false)}
          <span className="truncate">{session.title}</span>
        </CommandItem>)}
      </CommandGroup>
    </CommandList>
  </CommandDialog>;
}
