import {useEffect, useRef, useState} from 'react';
import {flushSync} from 'react-dom';
import {mentionFiles} from '../api.js';
import {Command, CommandEmpty, CommandInput, CommandItem, CommandList} from '../components/ui/command';
import {Dialog, DialogContent, DialogDescription, DialogTitle} from '../components/ui/dialog';
import {fileTabId, openFileTab} from '../editor/editor.js';
import {useStore} from '../store.js';

export function QuickOpen({open, onOpenChange}) {
  const [state, store] = useStore();
  const session = state.session?.id;
  const [files, setFiles] = useState([]);
  const [error, setError] = useState('');
  const [selected, setSelected] = useState('');
  const list = useRef(null);
  const invoker = useRef(null);
  const opened = useRef(null);
  useEffect(() => {
    if (!open || !session) return;
    const controller = new AbortController();
    setFiles([]); setError(''); setSelected('');
    mentionFiles({session}, controller.signal)
      .then(paths => { if (!controller.signal.aborted) setFiles(paths); })
      .catch(err => { if (!controller.signal.aborted) setError('Could not list files: ' + err.message); });
    return () => controller.abort();
  }, [open, session]);
  const keyDown = e => {
    if (!e.metaKey || e.ctrlKey || e.altKey || e.key.toLowerCase() !== 'p') return;
    e.preventDefault();
    const items = [...list.current.querySelectorAll('[cmdk-item]:not([aria-disabled="true"])')];
    if (!items.length) return;
    const index = items.findIndex(item => item.getAttribute('aria-selected') === 'true');
    const next = (index + (e.shiftKey ? -1 : 1) + items.length) % items.length;
    setSelected(items[next].getAttribute('data-value'));
  };
  return <Dialog open={open && !!session} onOpenChange={onOpenChange}>
    <DialogContent className="overflow-hidden p-0" showCloseButton={false}
      onOpenAutoFocus={() => { invoker.current = document.activeElement; opened.current = null; }}
      onCloseAutoFocus={e => {
        e.preventDefault();
        const tab = opened.current && [...document.querySelectorAll('[data-tab]')].find(element => element.dataset.tab === fileTabId(opened.current));
        (tab?.querySelector('[role="tab"]') ?? invoker.current)?.focus();
      }}>
      <DialogTitle className="sr-only">Quick Open</DialogTitle>
      <DialogDescription className="sr-only">Search session files</DialogDescription>
      <Command value={selected} onValueChange={setSelected} onKeyDown={keyDown} loop
        className="**:data-[slot=command-input-wrapper]:h-12 [&_[cmdk-group-heading]]:px-2 [&_[cmdk-group-heading]]:font-medium [&_[cmdk-group-heading]]:text-muted-foreground [&_[cmdk-group]]:px-2 [&_[cmdk-group]:not([hidden])_~[cmdk-group]]:pt-0 [&_[cmdk-input-wrapper]_svg]:h-5 [&_[cmdk-input-wrapper]_svg]:w-5 [&_[cmdk-input]]:h-12 [&_[cmdk-item]]:px-2 [&_[cmdk-item]]:py-3 [&_[cmdk-item]_svg]:h-5 [&_[cmdk-item]_svg]:w-5">
        <CommandInput placeholder="Search files by name" aria-label="Search files" />
        <CommandList ref={list}>
          {error ? <p className="p-4 text-sm">{error}</p> : <CommandEmpty>No files found.</CommandEmpty>}
          {files.map(path => <CommandItem key={path} value={path} onSelect={() => {
            opened.current = path;
            flushSync(() => { openFileTab(store, path); onOpenChange(false); });
          }}>{path}</CommandItem>)}
        </CommandList>
      </Command>
    </DialogContent>
  </Dialog>;
}
