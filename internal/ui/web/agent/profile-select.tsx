// Sessions choose configured profiles; a running session keeps its harness.
import {useEffect, useState} from 'react';
import {Button} from '../components/ui/button';
import {DropdownMenu, DropdownMenuContent, DropdownMenuRadioGroup, DropdownMenuRadioItem, DropdownMenuTrigger} from '../components/ui/dropdown-menu';
import {Icon} from '../shared/icon.tsx';
import {fetchProfiles, selectProfile} from '../api.ts';
import {unsentProfile} from '../sessions/selection.ts';
import {useStore} from '../store.ts';
import {unsentKey} from './drafts.ts';
import {modelLabel, profileLabel} from './profile-label.ts';
import {harnessIcon} from './harness-icon.tsx';
import type {Profile} from './types.ts';
import type {Session} from '../sessions/types.ts';

export function ProfileSelect({session}: {session: Session}) {
  const [state, store] = useStore();
  const unsent = session.id === unsentKey ? state.unsent : null;
  const [profiles, setProfiles] = useState<Profile[]>([]);
  useEffect(() => {
    if (!unsent && !session.ready) return;
    let current = true;
    fetchProfiles(store).then(fetched => {
      if (!current || !fetched?.length) return;
      setProfiles(fetched);
      if (unsent) store.update(s => s.unsent?.profile ? s : unsentProfile(s, fetched[0]));
    });
    return () => { current = false; };
  }, [store, session.id, session.ready, unsent?.identity]);
  const choose = (value: string) => {
    if (unsent ? unsent.sending : !session.ready) return;
    const profile = profiles.find(p => p.name === value);
    if (!profile) return;
    if (unsent) store.update(s => unsentProfile(s, profile));
    else selectProfile(store, session.id, profile.name);
  };
  const choices = unsent ? profiles : profiles.filter(p => p.harness === session.harness);
  const selected = unsent ? profiles.find(p => p.name === unsent.profile) : choices.find(p => p.model === session.model && p.effort === session.effort);
  const shown = unsent ? selected : session;
  const icon = shown && harnessIcon(shown.harness);
  return <DropdownMenu>
    <DropdownMenuTrigger asChild>
      <Button type="button" variant="ghost" size="sm" className="composer-profile h-6 min-w-0 max-w-full text-xs font-normal text-muted-foreground" aria-label="Profile" disabled={unsent ? unsent.sending : !session.ready}>
        {icon}<span className="truncate">{shown ? (icon ? modelLabel(shown) : profileLabel(shown)) : 'Select profile'}</span><Icon name="chevron-down" />
      </Button>
    </DropdownMenuTrigger>
    <DropdownMenuContent side="top" align="start" className="w-96 max-w-[calc(100vw-2rem)]">
      <DropdownMenuRadioGroup value={selected?.name ?? ''} onValueChange={choose}>
        {choices.map(p => {
          const icon = harnessIcon(p.harness);
          return <DropdownMenuRadioItem key={p.name} value={p.name}>
            {icon}<span className="min-w-0 whitespace-normal break-words">{icon ? modelLabel(p) : profileLabel(p)}{p.description && <span className="block text-xs text-muted-foreground">{p.description}</span>}</span>
          </DropdownMenuRadioItem>;
        })}
      </DropdownMenuRadioGroup>
    </DropdownMenuContent>
  </DropdownMenu>;
}
