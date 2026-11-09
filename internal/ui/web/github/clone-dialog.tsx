// The folder prompt for cloning a pull request's repository. The clone runs
// while the dialog is open; closing the dialog cancels it.
import {useEffect, useRef, useState, type FormEvent} from 'react';
import {cloneWorkspace} from '../api.ts';
import {hypercube} from '../app/window.ts';
import {Button} from '../components/ui/button';
import {Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle} from '../components/ui/dialog';
import {Input} from '../components/ui/input';
import {Label} from '../components/ui/label';
import {workspaceKey} from '../editor/target.ts';
import {selectSession} from '../sessions/selection.ts';
import {BusyDot} from '../shared/busy-dot.tsx';
import {useStore} from '../store.ts';
import {suggestedFolder} from './clone.ts';
import type {PullRequest} from './types.ts';

interface CloneDialogProps {
  pr: PullRequest;
  account: string;
  onClose: () => void;
}

export function CloneDialog({pr, account, onClose}: CloneDialogProps) {
  const [state, store] = useStore();
  const [folder, setFolder] = useState(() => suggestedFolder(state.workspaces, pr.repo));
  const [cloning, setCloning] = useState(false);
  const [error, setError] = useState('');
  const running = useRef<AbortController | null>(null);
  const name = pr.repo.split('/')[1];

  useEffect(() => () => running.current?.abort(), []);

  const close = () => { running.current?.abort(); onClose(); };

  const choose = async () => {
    const chosen = await hypercube()?.chooseFolder();
    if (chosen) setFolder(`${chosen.replace(/\/$/, '')}/${name}`);
  };

  async function clone(event: FormEvent) {
    event.preventDefault();
    const controller = new AbortController();
    running.current = controller;
    setCloning(true);
    setError('');
    const result = await cloneWorkspace({repo: pr.repo, account, path: folder.trim()}, controller.signal);
    if (controller.signal.aborted) return;
    running.current = null;
    if ('error' in result) {
      setCloning(false);
      setError(result.error);
      return;
    }
    store.update(s => selectSession(s, workspaceKey(result.workspace)));
    onClose();
  }

  return <Dialog open onOpenChange={open => { if (!open) close(); }}>
    <DialogContent>
      <form onSubmit={clone} className="grid gap-4">
        <DialogHeader>
          <DialogTitle>Clone {pr.repo}</DialogTitle>
          <DialogDescription>Clones with {account} and adds a workspace.</DialogDescription>
        </DialogHeader>
        <div className="grid gap-2">
          <Label htmlFor="clone-folder">Folder</Label>
          <div className="flex gap-2">
            <Input id="clone-folder" value={folder} disabled={cloning} onChange={event => setFolder(event.target.value)} />
            {hypercube() && <Button type="button" variant="secondary" disabled={cloning} onClick={choose}>Choose…</Button>}
          </div>
          {error && <p role="alert" className="text-sm break-words whitespace-pre-wrap text-destructive">{error}</p>}
        </div>
        <DialogFooter>
          <Button type="button" variant="secondary" onClick={close}>Cancel</Button>
          <Button type="submit" disabled={cloning || !folder.trim()}>
            {cloning ? <><BusyDot title="Cloning" />Cloning…</> : 'Clone'}
          </Button>
        </DialogFooter>
      </form>
    </DialogContent>
  </Dialog>;
}
