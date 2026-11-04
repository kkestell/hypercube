import {useEffect, useState} from 'react';
import {mentionFiles} from '../api.js';
import {CommandEmpty, CommandInput, CommandItem, CommandList} from '../components/ui/command';
import {openFileTab} from '../editor/editor.js';
import {CommandDialog} from '../shared/command-dialog.jsx';
import {useStore} from '../store.js';

export function QuickOpen({open, onOpenChange}) {
  const [state, store] = useStore();
  const session = state.session?.id;
  const [files, setFiles] = useState([]);
  const [error, setError] = useState('');
  useEffect(() => {
    if (!open || !session) return;
    const controller = new AbortController();
    setFiles([]); setError('');
    mentionFiles({session}, controller.signal)
      .then(paths => { if (!controller.signal.aborted) setFiles(paths); })
      .catch(err => { if (!controller.signal.aborted) setError('Could not list files: ' + err.message); });
    return () => controller.abort();
  }, [open, session]);
  const keyDown = e => {
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
