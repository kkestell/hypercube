// Quick Open: find a file of the current view by name and open it. The list
// loads each time the dialog opens and stays drawn while it closes.
import type {KeyboardEvent} from 'react';
import {mentionFiles} from '../api.ts';
import {CommandEmpty, CommandInput, CommandItem, CommandList} from '../components/ui/command';
import type {EditorTarget} from '../editor/target.ts';
import {openFileTab} from '../editor/editor.ts';
import {matches} from '../app/shortcuts.ts';
import {setDialog} from '../app/workbench.ts';
import {CommandDialog} from '../shared/command-dialog.tsx';
import {Message} from '../shared/message.tsx';
import {useRequest} from '../shared/use-request.ts';
import {useStore} from '../store.ts';

export function QuickOpen() {
  const [state, store] = useStore();
  const editor = state.editor;
  const open = state.dialog === 'quickOpen' && !!editor;
  const onOpenChange = (next: boolean) => store.update(s => setDialog(s, 'quickOpen', next));
  const keyDown = (e: KeyboardEvent) => {
    if (matches(e, 'quickOpen')) e.preventDefault();
  };
  return <CommandDialog open={open} onOpenChange={onOpenChange} title="Quick Open" description="Search files" commandProps={{onKeyDown: keyDown}}>
    <CommandInput placeholder="Search files by name" aria-label="Search files" />
    {editor && <Files target={editor.target} onOpen={path => { onOpenChange(false); openFileTab(store, path); }} />}
  </CommandDialog>;
}

// Files mounts with the dialog's content, so each opening lists them again.
function Files({target, onOpen}: {target: EditorTarget; onOpen: (path: string) => void}) {
  const {data: files = [], error, loading} = useRequest(signal => mentionFiles(target, signal), []);
  return <CommandList>
    {error ? <Message error className="p-4">Could not list files: {error}</Message>
      : loading ? <Message className="p-4">Loading files.</Message> : <CommandEmpty><Message>No files found.</Message></CommandEmpty>}
    {files.map(path => <CommandItem key={path} value={path} onSelect={() => onOpen(path)}>{path}</CommandItem>)}
  </CommandList>;
}
