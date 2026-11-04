import {useEffect, useState} from 'react';
import {Search} from 'lucide-react';
import {CommandEmpty, CommandInput, CommandItem, CommandList} from '../components/ui/command';
import {CommandDialog} from '../shared/command-dialog.jsx';
import {Button} from '../components/ui/button';
import {chooseSession} from '../sessions/session-actions.js';
import {useStore} from '../store.js';

export function SessionSearch() {
  const [, store] = useStore();
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState('');
  const [results, setResults] = useState([]);
  const [error, setError] = useState('');

  useEffect(() => {
    const key = event => {
      if (event.metaKey && event.key.toLowerCase() === 'k') {
        event.preventDefault();
        setOpen(true);
      }
    };
    window.addEventListener('keydown', key);
    return () => window.removeEventListener('keydown', key);
  }, []);

  useEffect(() => {
    if (!open || !query.trim()) {
      setResults([]);
      setError('');
      return;
    }
    const controller = new AbortController();
    setResults([]);
    setError('');
    fetch('search-sessions?' + new URLSearchParams({q: query}), {signal: controller.signal})
      .then(response => {
        if (!response.ok) throw new Error('Could not search sessions');
        return response.json();
      })
      .then(data => setResults(data ?? []))
      .catch(err => { if (err.name !== 'AbortError') setError(err.message); });
    return () => controller.abort();
  }, [open, query]);

  const changeOpen = next => {
    setOpen(next);
    if (!next) setQuery('');
  };
  return <>
    <Button variant="ghost" size="icon" aria-label="Search sessions" title="Search sessions" onClick={() => setOpen(true)}><Search /></Button>
    <CommandDialog open={open} onOpenChange={changeOpen} title="Search sessions" description="Search session titles and conversations" commandProps={{shouldFilter: false}}>
      <CommandInput placeholder="Search sessions..." value={query} onValueChange={setQuery} />
      <CommandList>
        {query.trim() && <CommandEmpty>{error || 'No sessions found.'}</CommandEmpty>}
        {results.map(result => <CommandItem key={result.id} value={`${result.id} ${result.title} ${result.text ?? ''}`} onSelect={() => { changeOpen(false); chooseSession(store, result.id); }}>
          <div className="flex min-w-0 flex-col gap-1">
            <span className="truncate">{result.title}</span>
            <span className="truncate text-xs text-muted-foreground">{result.workspace}</span>
            {result.text && <span className="line-clamp-2 text-xs text-muted-foreground">{result.text}</span>}
          </div>
        </CommandItem>)}
      </CommandList>
    </CommandDialog>
  </>;
}
