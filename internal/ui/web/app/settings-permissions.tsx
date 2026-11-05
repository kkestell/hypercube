import {useEffect, useState} from 'react';
import {fetchGrants, post} from '../api.ts';
import {Button} from '../components/ui/button';
import {useStore} from '../store.ts';
import type {Grant} from './types.ts';

export function SettingsPermissions() {
  const [, store] = useStore();
  const [grants, setGrants] = useState<Grant[] | null>(null);
  useEffect(() => {
    let active = true;
    fetchGrants(store).then(result => { if (active && result) setGrants(result); });
    return () => { active = false; };
  }, [store]);
  const remove = async (grant: Grant) => {
    if (await post(store, 'grants', {session: grant.session, harness: grant.harness, tool: grant.tool}) !== null) {
      setGrants(items => items!.filter(item => item !== grant));
    }
  };
  return <section className="grid gap-4">
    <p className="text-sm text-muted-foreground">Review tools you previously chose to always allow. Removing a choice lets the harness ask again when approval is required. Changes apply immediately.</p>
    <h3 className="font-semibold">Always Allowed Tools</h3>
    {grants === null ? <p>Loading saved choices…</p> : grants.length === 0 ? <p>No saved choices.</p> : <ul className="divide-y rounded-lg border">
      {grants.map(grant => <li key={JSON.stringify([grant.session, grant.harness, grant.tool])} className="flex items-center justify-between gap-4 p-3">
        <div><div>{grant.tool}</div><div className="text-sm text-muted-foreground">{grant.harness} · {grant.session ? grant.title : 'All sessions'}</div></div>
        <Button variant="outline" size="sm" aria-label={`Remove ${grant.tool}, ${grant.harness}, ${grant.session ? grant.title : 'all sessions'}`} onClick={() => remove(grant)}>Remove</Button>
      </li>)}
    </ul>}
  </section>;
}
