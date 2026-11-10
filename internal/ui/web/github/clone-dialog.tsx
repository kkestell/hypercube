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
import {useDialogFocus} from '../shared/focus.ts';
import {Message} from '../shared/message.tsx';
import {useStore} from '../store.ts';
import {suggestedFolder} from './clone.ts';
import type {PullRequest} from './types.ts';

interface CloneDialogProps {
  open: boolean;
  pr: PullRequest;
  account: string;
  onClose: () => void;
}

// The dialog stays mounted so it can animate closed; its form starts afresh
// each time it opens.
export function CloneDialog({open, ...props}: CloneDialogProps) {
  const focus = useDialogFocus(open);
  return <Dialog open={open} onOpenChange={next => { if (!next) props.onClose(); }}>
    <DialogContent {...focus}>
      <CloneForm open={open} {...props} />
    </DialogContent>
  </Dialog>;
}

// CloneForm mounts with the dialog's content. Closing cancels a clone at once,
// before the dialog finishes animating closed.
function CloneForm({open, pr, account, onClose}: CloneDialogProps) {
  const [state, store] = useStore();
  const [folder, setFolder] = useState(() => suggestedFolder(state.workspaces, pr.repo));
  const [cloning, setCloning] = useState(false);
  const [error, setError] = useState('');
  const running = useRef<AbortController | null>(null);
  const name = pr.repo.split('/')[1];

  useEffect(() => {
    if (!open) running.current?.abort();
  }, [open]);
  useEffect(() => () => running.current?.abort(), []);
  const cancel = () => { running.current?.abort(); onClose(); };

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

  return <form onSubmit={clone} className="grid gap-4">
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
      {error && <Message error>{error}</Message>}
    </div>
    <DialogFooter>
      <Button type="button" variant="outline" onClick={cancel}>Cancel</Button>
      <Button type="submit" disabled={cloning || !folder.trim()}>
        {cloning ? <><BusyDot title="Cloning" />Cloning…</> : 'Clone'}
      </Button>
    </DialogFooter>
  </form>;
}
