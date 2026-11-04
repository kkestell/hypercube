// A new session chooses a configured profile; existing sessions show its label.
import {useEffect, useState} from 'react';
import {Select, SelectContent, SelectItem, SelectTrigger, SelectValue} from '../components/ui/select';
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
  return <Select value={unsent.profile || undefined} disabled={unsent.sending} onValueChange={choose}>
    <SelectTrigger className="composer-profile" aria-label="Profile"><SelectValue placeholder="Select profile" /></SelectTrigger>
    <SelectContent side="top">
      {profiles.map(p => <SelectItem key={p.name} value={p.name} textValue={profileLabel(p)}>
        <span>{profileLabel(p)}{p.description && <span className="block text-xs text-muted-foreground">{p.description}</span>}</span>
      </SelectItem>)}
    </SelectContent>
  </Select>;
}
