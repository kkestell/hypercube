// Search across session titles and conversations, from its button in the
// sidebar or its shortcut. Choosing a result shows its session.
import {useState} from 'react';
import {searchSessions} from '../api.ts';
import {setDialog} from '../app/workbench.ts';
import {CommandEmpty, CommandInput, CommandItem, CommandList} from '../components/ui/command';
import {chooseSession} from '../sessions/session-actions.ts';
import {CommandDialog} from '../shared/command-dialog.tsx';
import {IconButton} from '../shared/icon-button.tsx';
import {useRequest} from '../shared/use-request.ts';
import {useStore} from '../store.ts';

export function SessionSearch() {
  const [state, store] = useStore();
  const open = state.dialog === 'sessionSearch';
  const [query, setQuery] = useState('');
  const typed = query.trim();
  const search = useRequest(open && typed ? signal => searchSessions(query, signal) : null, [open, query]);

  const changeOpen = (next: boolean) => {
    store.update(s => setDialog(s, 'sessionSearch', next));
    if (!next) setQuery('');
  };
  return <>
    <IconButton icon="search" label="Search sessions" shortcut="searchSessions" onClick={() => changeOpen(true)} />
    <CommandDialog open={open} onOpenChange={changeOpen} title="Search sessions" description="Search session titles and conversations" commandProps={{shouldFilter: false}}>
      <CommandInput placeholder="Search sessions..." value={query} onValueChange={setQuery} />
      <CommandList>
        {typed && <CommandEmpty>{search.error ? 'Could not search sessions: ' + search.error : search.loading ? 'Searching…' : 'No sessions found.'}</CommandEmpty>}
        {typed && search.data?.map(result => <CommandItem key={result.id} value={`${result.id} ${result.title} ${result.text ?? ''}`} onSelect={() => { changeOpen(false); chooseSession(store, result.id); }}>
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
