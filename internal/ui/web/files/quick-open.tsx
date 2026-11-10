import type {KeyboardEvent} from 'react';
import {mentionFiles} from '../api.ts';
import {CommandEmpty, CommandInput, CommandItem, CommandList} from '../components/ui/command';
import {openFileTab} from '../editor/editor.ts';
import {matches} from '../app/shortcuts.ts';
import {setDialog} from '../app/workbench.ts';
import {CommandDialog} from '../shared/command-dialog.tsx';
import {Message} from '../shared/message.tsx';
import {useRequest} from '../shared/use-request.ts';
import {useStore} from '../store.ts';

export function QuickOpen() {
  const [state, store] = useStore();
  const open = state.dialog === 'quickOpen' && !state.unsent;
  const onOpenChange = (next: boolean) => store.update(s => setDialog(s, 'quickOpen', next));
  const editor = state.editor?.id;
  const {data: files = [], error, loading} = useRequest(open && editor ? signal => mentionFiles(state.editor!.target, signal) : null, [open, editor]);
  const keyDown = (e: KeyboardEvent) => {
    if (matches(e, 'quickOpen')) e.preventDefault();
  };
  return <CommandDialog open={open && !!editor} onOpenChange={onOpenChange} title="Quick Open" description="Search files" commandProps={{onKeyDown: keyDown}}>
    <CommandInput placeholder="Search files by name" aria-label="Search files" />
    <CommandList>
      {error ? <Message error className="p-4">Could not list files: {error}</Message>
        : loading ? <Message className="p-4">Loading files.</Message> : <CommandEmpty><Message>No files found.</Message></CommandEmpty>}
      {files.map(path => <CommandItem key={path} value={path} onSelect={() => { onOpenChange(false); openFileTab(store, path); }}>{path}</CommandItem>)}
    </CommandList>
  </CommandDialog>;
}
