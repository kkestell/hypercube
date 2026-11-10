// The prompt for a new worktree session's branch name and the branch, commit,
// or ref it starts from. Nothing is created until the session is sent.
import {useEffect, useState, type FormEvent} from 'react';
import {loadBranches} from '../api.ts';
import {Button} from '../components/ui/button';
import {Command, CommandGroup, CommandInput, CommandItem, CommandList} from '../components/ui/command';
import {Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle} from '../components/ui/dialog';
import {Input} from '../components/ui/input';
import {Label} from '../components/ui/label';
import {Popover, PopoverContent, PopoverTrigger} from '../components/ui/popover';
import type {BranchList, NewBranch} from '../sessions/types.ts';
import {Icon} from '../shared/icon.tsx';

interface NewBranchDialogProps {
  workspace: string;
  // current is the branch already chosen for this session, if any.
  current: NewBranch | null;
  onSave: (branch: NewBranch) => void;
  onClose: () => void;
}

export function NewBranchDialog({workspace, current, onSave, onClose}: NewBranchDialogProps) {
  const [branches, setBranches] = useState<BranchList | null>(null);
  const [error, setError] = useState('');
  const [name, setName] = useState(current?.name ?? '');
  const [start, setStart] = useState(current?.start ?? '');

  useEffect(() => {
    const controller = new AbortController();
    loadBranches(workspace, controller.signal).then(setBranches, reason => {
      if (!controller.signal.aborted) setError(reason instanceof Error ? reason.message : String(reason));
    });
    return () => controller.abort();
  }, [workspace]);

  const trimmed = name.trim();
  const taken = !!branches && branches.local.includes(trimmed);
  const source = start || branches?.current || 'HEAD';

  const save = (event: FormEvent) => {
    event.preventDefault();
    onSave({name: trimmed, start: source});
  };

  return <Dialog open onOpenChange={open => { if (!open) onClose(); }}>
    <DialogContent>
      <form onSubmit={save} className="grid gap-4">
        <DialogHeader>
          <DialogTitle>Create a branch</DialogTitle>
        </DialogHeader>
        <div className="grid gap-2">
          <Label htmlFor="new-branch-name">New branch name</Label>
          <Input id="new-branch-name" value={name} autoFocus onChange={event => setName(event.target.value)} />
          {taken && <p role="alert" className="text-sm text-destructive">A branch named {trimmed} already exists.</p>}
        </div>
        <div className="grid gap-2">
          <Label htmlFor="new-branch-source">Source</Label>
          <SourceSelect branches={branches} value={source} onChange={setStart} />
          {error && <p role="alert" className="text-sm break-words whitespace-pre-wrap text-destructive">{error}</p>}
        </div>
        <DialogFooter>
          <Button type="button" variant="secondary" onClick={onClose}>Cancel</Button>
          <Button type="submit" disabled={!branches || !trimmed || taken}>Save</Button>
        </DialogFooter>
      </form>
    </DialogContent>
  </Dialog>;
}

// SourceSelect picks a listed branch, or any typed commit or ref.
function SourceSelect({branches, value, onChange}: {branches: BranchList | null; value: string; onChange: (start: string) => void}) {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState('');
  const typed = query.trim();
  const listed = !!branches && (branches.local.includes(typed) || branches.remote.includes(typed));
  const choose = (start: string) => { onChange(start); setOpen(false); };
  const item = (branch: string) => <CommandItem key={branch} value={branch} onSelect={() => choose(branch)}>
    <span className="min-w-0 flex-1 truncate">{branch}</span><Icon name="check" className={branch === value ? '' : 'invisible'} />
  </CommandItem>;
  return <Popover open={open} onOpenChange={next => { setOpen(next); setQuery(''); }}>
    <PopoverTrigger asChild>
      <Button id="new-branch-source" type="button" variant="outline" className="justify-between font-normal" disabled={!branches}>
        <Icon name="git-branch" /><span className="min-w-0 flex-1 truncate text-left">{value}</span><Icon name="chevron-down" />
      </Button>
    </PopoverTrigger>
    <PopoverContent align="start" className="w-(--radix-popover-trigger-width) p-0">
      <Command loop>
        <CommandInput value={query} onValueChange={setQuery} placeholder="Search branches or enter a commit" aria-label="Search branches or enter a commit" />
        <CommandList className="max-h-64">
          {typed && !listed && <CommandItem value={`use ${typed}`} forceMount onSelect={() => choose(typed)}>Use “{typed}”</CommandItem>}
          {!!branches?.local.length && <CommandGroup heading="Branches">{branches.local.map(item)}</CommandGroup>}
          {!!branches?.remote.length && <CommandGroup heading="Remote branches">{branches.remote.map(item)}</CommandGroup>}
        </CommandList>
      </Command>
    </PopoverContent>
  </Popover>;
}
