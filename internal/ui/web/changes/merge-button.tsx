// Ask the session's agent to commit its remaining work and merge its branch into the checkout.
import {sessionOf} from '../sessions/selection.ts';
import {requestMerge} from '../sessions/session-actions.ts';
import type {Session} from '../sessions/types.ts';
import {Button} from '../components/ui/button';
import {useStore} from '../store.ts';
import {Icon} from '../shared/icon.tsx';

export function MergeButton({session}: {session: Session}) {
  const [state, store] = useStore();
  const {merge} = session;
  const found = sessionOf(state, session.id);
  if (!merge || !found || merge.unmerged <= 0 || merge.error) return null;

  return (
    <div className="shrink-0 border-t p-2">
      <Button type="button" variant="secondary" size="sm" className="w-full"
        title={`Ask the agent to merge this branch into ${merge.target}`}
        onClick={() => requestMerge(store, session, merge.target, found.workspace.path)}>
        <Icon name="git-merge" />
        Merge into {merge.target}
      </Button>
    </div>
  );
}
