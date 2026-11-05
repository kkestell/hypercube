import {useEffect, useState} from 'react';
import {fetchGrants, fetchVersion, post} from '../api.ts';
import {Button} from '../components/ui/button';
import {Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle} from '../components/ui/dialog';
import {useModal} from '../shared/overlays.ts';
import {useStore} from '../store.ts';
import type {Grant} from './types.ts';

export function SettingsDialog({open, onOpenChange}: {open: boolean; onOpenChange: (open: boolean) => void}) {
  const [, store] = useStore();
  const [grants, setGrants] = useState<Grant[]>([]);
  const [version, setVersion] = useState('');
  const focus = useModal(open);
  useEffect(() => {
    if (!open) return;
    let active = true;
    setGrants([]);
    setVersion('');
    fetchGrants(store).then(result => { if (active && result) setGrants(result); });
    fetchVersion(store).then(result => { if (active && result) setVersion(result); });
    return () => { active = false; };
  }, [open, store]);
  const remove = async (grant: Grant) => {
    if (await post(store, 'grants', {session: grant.session, harness: grant.harness, tool: grant.tool}) !== null) {
      setGrants(items => items.filter(item => item !== grant));
    }
  };
  return <Dialog open={open} onOpenChange={onOpenChange}>
    <DialogContent {...focus} className="max-h-[80vh] overflow-y-auto">
      <DialogHeader><DialogTitle>Settings</DialogTitle>
        <DialogDescription className="sr-only">Manage saved choices and view build information.</DialogDescription></DialogHeader>
      <section><h2 className="mb-2 font-semibold">Always Allowed Tools</h2>
        {grants.length === 0 ? <p>No saved choices.</p> : <ul className="divide-y rounded-lg border">
          {grants.map(grant => <li key={JSON.stringify([grant.session, grant.harness, grant.tool])} className="flex items-center justify-between gap-4 p-3">
            <div><div>{grant.tool}</div><div className="text-sm text-muted-foreground">{grant.harness} · {grant.session ? grant.title : 'All sessions'}</div></div>
            <Button variant="outline" size="sm" aria-label={`Remove ${grant.tool}, ${grant.harness}, ${grant.session ? grant.title : 'all sessions'}`} onClick={() => remove(grant)}>Remove</Button>
          </li>)}
        </ul>}
      </section>
      <section aria-label="Build"><h2 className="mb-2 font-semibold">Build</h2>
        <p className="break-all text-sm text-muted-foreground">{version}</p>
      </section>
    </DialogContent>
  </Dialog>;
}
