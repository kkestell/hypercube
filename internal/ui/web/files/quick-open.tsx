import {useEffect, useState, type KeyboardEvent} from 'react';
import {mentionFiles} from '../api.ts';
import {CommandEmpty, CommandInput, CommandItem, CommandList} from '../components/ui/command';
import {openFileTab} from '../editor/editor.ts';
import {CommandDialog} from '../shared/command-dialog.tsx';
import {useStore} from '../store.ts';

interface QuickOpenProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export function QuickOpen({open, onOpenChange}: QuickOpenProps) {
  const [state, store] = useStore();
  const session = state.session?.id;
  const [files, setFiles] = useState<string[]>([]);
  const [error, setError] = useState('');
  useEffect(() => {
    if (!open || !session) return;
    const controller = new AbortController();
    setFiles([]); setError('');
    mentionFiles({session}, controller.signal)
      .then(paths => { if (!controller.signal.aborted) setFiles(paths); })
      .catch((err: unknown) => { if (!controller.signal.aborted) setError('Could not list files: ' + (err instanceof Error ? err.message : String(err))); });
    return () => controller.abort();
  }, [open, session]);
  const keyDown = (e: KeyboardEvent) => {
    if (e.metaKey && !e.ctrlKey && !e.altKey && e.key.toLowerCase() === 'p') e.preventDefault();
  };
  return <CommandDialog open={open && !!session} onOpenChange={onOpenChange} title="Quick Open" description="Search session files" commandProps={{onKeyDown: keyDown}}>
    <CommandInput placeholder="Search files by name" aria-label="Search files" />
    <CommandList>
      {error ? <p className="p-4 text-sm">{error}</p> : <CommandEmpty>No files found.</CommandEmpty>}
      {files.map(path => <CommandItem key={path} value={path} onSelect={() => { onOpenChange(false); openFileTab(store, path); }}>{path}</CommandItem>)}
    </CommandList>
  </CommandDialog>;
}
