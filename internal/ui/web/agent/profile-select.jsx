// A new session chooses a configured profile; existing sessions show its label.
import {useEffect, useState} from 'react';
import {Button} from '../components/ui/button';
import {DropdownMenu, DropdownMenuContent, DropdownMenuRadioGroup, DropdownMenuRadioItem, DropdownMenuTrigger} from '../components/ui/dropdown-menu';
import {Icon} from '../shared/icon.jsx';
import {fetchProfiles} from '../api.js';
import {unsentProfile} from '../sessions/selection.js';
import {useStore} from '../store.js';
import {unsentKey} from './drafts.js';
import {profileLabel} from './profile-label.js';

export function ProfileSelect({session}) {
  const [state, store] = useStore();
  const unsent = session.id === unsentKey ? state.unsent : null;
  const [profiles, setProfiles] = useState([]);
  useEffect(() => {
    if (!unsent) return;
    let current = true;
    fetchProfiles(store).then(fetched => {
      if (!current || !fetched?.length) return;
      setProfiles(fetched);
      store.update(s => s.unsent?.profile ? s : unsentProfile(s, fetched[0]));
    });
    return () => { current = false; };
  }, [unsent?.identity]);
  const choose = value => {
    if (!unsent || unsent.sending) return;
    const profile = profiles.find(p => p.name === value);
    if (profile) store.update(s => unsentProfile(s, profile));
  };
  if (!unsent) return <span className="composer-profile-label truncate text-muted-foreground">{profileLabel(session)}</span>;
  const selected = profiles.find(p => p.name === unsent.profile);
  return <DropdownMenu>
    <DropdownMenuTrigger asChild>
      <Button type="button" variant="ghost" size="sm" className="composer-profile min-w-0 max-w-full" aria-label="Profile" disabled={unsent.sending}>
        <span className="truncate">{selected ? profileLabel(selected) : 'Select profile'}</span><Icon name="chevron-down" />
      </Button>
    </DropdownMenuTrigger>
    <DropdownMenuContent side="top" align="start" className="w-96 max-w-[calc(100vw-2rem)]">
      <DropdownMenuRadioGroup value={unsent.profile ?? ''} onValueChange={choose}>
        {profiles.map(p => <DropdownMenuRadioItem key={p.name} value={p.name}>
          <span className="min-w-0 whitespace-normal break-words">{profileLabel(p)}{p.description && <span className="block text-xs text-muted-foreground">{p.description}</span>}</span>
        </DropdownMenuRadioItem>)}
      </DropdownMenuRadioGroup>
    </DropdownMenuContent>
  </DropdownMenu>;
}
