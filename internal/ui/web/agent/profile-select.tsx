// Sessions choose configured profiles; a running session keeps its harness.
import {useEffect, useState} from 'react';
import {Button} from '../components/ui/button';
import {DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger} from '../components/ui/dropdown-menu';
import {Icon} from '../shared/icon.tsx';
import {fetchConfig, selectProfile} from '../api.ts';
import {receivedConfig} from '../app/config.ts';
import {unsentProfile} from '../sessions/selection.ts';
import {useStore} from '../store.ts';
import {isUnsent} from './drafts.ts';
import {modelLabel, profileLabel} from './profile-label.ts';
import {harnessIcon} from './harness-icon.tsx';
import type {Session} from '../sessions/types.ts';
import type {Profile} from './types.ts';

export function ProfileSelect({session}: {session: Session}) {
  const [state, store] = useStore();
  const unsent = isUnsent(session.id) ? state.unsent : null;
  const profiles = state.config?.profiles ?? [];
  const [revision, refresh] = useState(0);
  useEffect(() => {
    if (!unsent && !session.ready) return;
    let current = true;
    fetchConfig(store).then(config => {
      if (!current || !config) return;
      store.update(s => receivedConfig(s, config));
    });
    return () => { current = false; };
  }, [store, session.id, session.ready, unsent?.identity, revision]);
  const choose = (profile: Profile) => {
    if (unsent ? unsent.sending : !session.ready) return;
    if (unsent) store.update(s => unsentProfile(s, profile));
    else selectProfile(store, session.id, profile.name);
  };
  const choices = unsent ? profiles : profiles.filter(p => p.harness === session.harness);
  const selected = unsent ? profiles.find(p => p.name === unsent.profile) : choices.find(p => p.model === session.model && p.effort === session.effort);
  const shown = unsent ? selected : session;
  const icon = shown && harnessIcon(shown.harness);
  return <DropdownMenu onOpenChange={open => { if (open) refresh(value => value + 1); }}>
    <DropdownMenuTrigger asChild>
      <Button type="button" variant="ghost" size="sm" className="composer-profile h-6 min-w-0 max-w-full text-xs font-normal text-muted-foreground" aria-label="Profile" disabled={unsent ? unsent.sending : !session.ready}>
        {icon}<span className="truncate">{shown ? (icon ? modelLabel(shown) : profileLabel(shown)) : 'Select profile'}</span><Icon name="chevron-down" />
      </Button>
    </DropdownMenuTrigger>
    <DropdownMenuContent side="top" align="start" className="min-w-56 max-w-[calc(100vw-2rem)]">
      {choices.map(p => {
        const icon = harnessIcon(p.harness);
        return <DropdownMenuItem key={p.name} role="menuitemradio" aria-checked={p === selected} onSelect={() => choose(p)}>
          {icon}<span className="min-w-0 truncate">{icon ? modelLabel(p) : profileLabel(p)}</span>
          {p === selected && <Icon name="check" className="ml-auto" />}
        </DropdownMenuItem>;
      })}
    </DropdownMenuContent>
  </DropdownMenu>;
}
