// The composer's profile choice for a new session, which defaults to the first
// profile and disables while the first prompt sends. Each option shows its
// harness, model, and effort, with its description beneath. Existing sessions
// show their saved configuration as plain text.
import {useEffect, useState} from 'preact/hooks';
import {fetchProfiles} from '../api.js';
import {unsentProfile} from '../sessions/selection.js';
import {useStore} from '../store.js';
import {unsentKey} from './drafts.js';
import {profileLabel} from './profile-label.js';

export function ProfileSelect({session}) {
  const [state, store] = useStore();
  const unsent = session.id === unsentKey ? state.unsent : null;
  const [profiles, setProfiles] = useState([]);

  // A fetch for an unsent session that has since been replaced is dropped.
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

  const choose = e => {
    if (!unsent || unsent.sending) return;
    const profile = profiles.find(p => p.name === e.currentTarget.value);
    if (profile) store.update(s => unsentProfile(s, profile));
  };

  if (!unsent) return <span class="composer-profile-label">{profileLabel(session)}</span>;
  return (
    <vscode-single-select class="composer-profile" label="Profile" position="above" disabled={unsent.sending} onChange={choose}>
      {profiles.map(p => (
        <vscode-option key={p.name} value={p.name} description={p.description} selected={p.name === unsent.profile}>
          {profileLabel(p)}
        </vscode-option>
      ))}
    </vscode-single-select>
  );
}
