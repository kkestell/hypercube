// The Permissions section: the tools the user chose to always allow, each
// with a button that removes the choice at once.
import {useState} from 'react';
import {fetchGrants, post} from '../api.ts';
import {Button} from '../components/ui/button';
import {harnessIcon} from '../agent/harness-icon.tsx';
import {displayName} from '../agent/selection-label.ts';
import {Message} from '../shared/message.tsx';
import {useRequest} from '../shared/use-request.ts';
import {useStore} from '../store.ts';
import type {Grant} from './types.ts';

export function SettingsPermissions() {
  const [, store] = useStore();
  const loaded = useRequest(() => fetchGrants(store), [store]);
  const [removed, setRemoved] = useState<Grant[]>([]);
  const grants = loaded.data?.filter(grant => !removed.includes(grant));
  const remove = async (grant: Grant) => {
    if (await post(store, 'grants', {session: grant.session, harness: grant.harness, tool: grant.tool}) !== null) setRemoved(items => [...items, grant]);
  };
  return <section className="grid gap-4">
    <p className="text-sm text-muted-foreground">Review tools you previously chose to always allow. Removing a choice lets the harness ask again when approval is required. Changes apply immediately.</p>
    <h3 className="text-sm font-semibold">Always Allowed Tools</h3>
    {!grants ? <Message error={loaded.data === null}>{loaded.data === null ? 'Could not load saved choices.' : 'Loading saved choices…'}</Message>
      : grants.length === 0 ? <Message>No saved choices.</Message> : <ul className="divide-y rounded-md border">
      {grants.map(grant => <li key={JSON.stringify([grant.session, grant.harness, grant.tool])} className="flex items-center justify-between gap-4 p-3">
        <div><div>{grant.tool}</div><div className="flex items-center gap-2 text-sm text-muted-foreground">{harnessIcon(grant.harness) ?? displayName(grant.harness)}<span>{grant.session ? grant.title : 'All sessions'}</span></div></div>
        <Button variant="outline" size="sm" aria-label={`Remove ${grant.tool}, ${displayName(grant.harness)}, ${grant.session ? grant.title : 'all sessions'}`} onClick={() => remove(grant)}>Remove</Button>
      </li>)}
    </ul>}
  </section>;
}
