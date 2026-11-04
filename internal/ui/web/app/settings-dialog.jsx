import {useEffect, useState} from 'react';
import {fetchGrants, post} from '../api.js';
import {Button} from '../components/ui/button';
import {Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle} from '../components/ui/dialog';
import {useModal} from '../shared/overlays.js';
import {useStore} from '../store.js';

export function SettingsDialog({open, onOpenChange}) {
  const [, store] = useStore();
  const [grants, setGrants] = useState([]);
  const focus = useModal(open);
  useEffect(() => {
    if (!open) return;
    let active = true;
    setGrants([]);
    fetchGrants(store).then(result => { if (active && result) setGrants(result); });
    return () => { active = false; };
  }, [open, store]);
  const remove = async grant => {
    if (await post(store, 'grants', {session: grant.session, harness: grant.harness, tool: grant.tool}) !== null) {
      setGrants(items => items.filter(item => item !== grant));
    }
  };
  return <Dialog open={open} onOpenChange={onOpenChange}>
    <DialogContent {...focus} className="max-h-[80vh] overflow-y-auto">
      <DialogHeader><DialogTitle>Settings</DialogTitle>
        <DialogDescription className="sr-only">Manage saved choices.</DialogDescription></DialogHeader>
      <section><h2 className="mb-2 font-semibold">Always Allowed Tools</h2>
        {grants.length === 0 ? <p>No saved choices.</p> : <ul className="divide-y rounded-lg border">
          {grants.map(grant => <li key={JSON.stringify([grant.session, grant.harness, grant.tool])} className="flex items-center justify-between gap-4 p-3">
            <div><div>{grant.tool}</div><div className="text-sm text-muted-foreground">{grant.harness} · {grant.session ? grant.title : 'All sessions'}</div></div>
            <Button variant="outline" size="sm" aria-label={`Remove ${grant.tool}, ${grant.harness}, ${grant.session ? grant.title : 'all sessions'}`} onClick={() => remove(grant)}>Remove</Button>
          </li>)}
        </ul>}
      </section>
    </DialogContent>
  </Dialog>;
}
