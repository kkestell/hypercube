import {useEffect, useState} from 'react';
import {Search} from 'lucide-react';
import {searchSessions} from '../api.ts';
import {CommandEmpty, CommandInput, CommandItem, CommandList} from '../components/ui/command';
import {CommandDialog} from '../shared/command-dialog.tsx';
import {Button} from '../components/ui/button';
import type {SessionSearchResult} from '../sessions/types.ts';

interface SessionSearchProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSelect: (id: string) => void;
}

export function SessionSearch({open, onOpenChange, onSelect}: SessionSearchProps) {
  const [query, setQuery] = useState('');
  const [results, setResults] = useState<SessionSearchResult[]>([]);
  const [error, setError] = useState('');

  useEffect(() => {
    if (!open || !query.trim()) {
      setResults([]);
      setError('');
      return;
    }
    const controller = new AbortController();
    setResults([]);
    setError('');
    searchSessions(query, controller.signal)
      .then(data => { if (!controller.signal.aborted) setResults(data); })
      .catch((err: unknown) => { if (!controller.signal.aborted) setError('Could not search sessions: ' + (err instanceof Error ? err.message : String(err))); });
    return () => controller.abort();
  }, [open, query]);

  const changeOpen = (next: boolean) => {
    onOpenChange(next);
    if (!next) setQuery('');
  };
  return <>
    <Button className="size-7" variant="ghost" size="icon" aria-label="Search sessions" title="Search sessions" onClick={() => onOpenChange(true)}><Search /></Button>
    <CommandDialog open={open} onOpenChange={changeOpen} title="Search sessions" description="Search session titles and conversations" commandProps={{shouldFilter: false}}>
      <CommandInput placeholder="Search sessions..." value={query} onValueChange={setQuery} />
      <CommandList>
        {query.trim() && <CommandEmpty>{error || 'No sessions found.'}</CommandEmpty>}
        {results.map(result => <CommandItem key={result.id} value={`${result.id} ${result.title} ${result.text ?? ''}`} onSelect={() => { changeOpen(false); onSelect(result.id); }}>
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
