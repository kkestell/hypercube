import {useEffect, useState, type KeyboardEvent} from 'react';
import {mentionFiles} from '../api.ts';
import {CommandEmpty, CommandInput, CommandItem, CommandList} from '../components/ui/command';
import {openFileTab} from '../editor/editor.ts';
import {matches} from '../app/shortcuts.ts';
import {setDialog} from '../app/workbench.ts';
import {CommandDialog} from '../shared/command-dialog.tsx';
import {useStore} from '../store.ts';

export function QuickOpen() {
  const [state, store] = useStore();
  const open = state.dialog === 'quickOpen' && !state.unsent;
  const onOpenChange = (next: boolean) => store.update(s => setDialog(s, 'quickOpen', next));
  const editor = state.editor?.id;
  const [files, setFiles] = useState<string[]>([]);
  const [error, setError] = useState('');
  useEffect(() => {
    if (!open || !editor) return;
    const controller = new AbortController();
    setFiles([]); setError('');
    mentionFiles(state.editor!.target, controller.signal)
      .then(paths => { if (!controller.signal.aborted) setFiles(paths); })
      .catch((err: unknown) => { if (!controller.signal.aborted) setError('Could not list files: ' + (err instanceof Error ? err.message : String(err))); });
    return () => controller.abort();
  }, [open, editor]);
  const keyDown = (e: KeyboardEvent) => {
    if (matches(e, 'quickOpen')) e.preventDefault();
  };
  return <CommandDialog open={open && !!editor} onOpenChange={onOpenChange} title="Quick Open" description="Search files" commandProps={{onKeyDown: keyDown}}>
    <CommandInput placeholder="Search files by name" aria-label="Search files" />
    <CommandList>
      {error ? <p className="p-4 text-sm">{error}</p> : <CommandEmpty>No files found.</CommandEmpty>}
      {files.map(path => <CommandItem key={path} value={path} onSelect={() => { onOpenChange(false); openFileTab(store, path); }}>{path}</CommandItem>)}
    </CommandList>
  </CommandDialog>;
}
