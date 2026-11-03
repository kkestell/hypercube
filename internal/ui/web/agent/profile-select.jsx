// The unsent session's profile choice in the composer: each option shows its
// harness, model, and effort, with the profile's description beneath. The
// first profile is chosen once the profiles arrive.
import {useEffect, useState} from 'preact/hooks';
import {fetchProfiles} from '../api.js';
import {unsentProfile} from '../sessions/selection.js';
import {useStore} from '../store.js';

export function ProfileSelect({unsent}) {
  const [, store] = useStore();
  const [profiles, setProfiles] = useState([]);

  // A fetch for an unsent session that has since been replaced is dropped.
  useEffect(() => {
    let current = true;
    fetchProfiles(store).then(fetched => {
      if (!current || !fetched?.length) return;
      setProfiles(fetched);
      store.update(s => s.unsent?.profile ? s : unsentProfile(s, fetched[0]));
    });
    return () => { current = false; };
  }, [unsent.identity]);

  const choose = e => {
    const profile = profiles.find(p => p.name === e.currentTarget.value);
    if (profile) store.update(s => unsentProfile(s, profile));
  };

  return (
    <vscode-single-select class="composer-profile" label="Profile" position="above" disabled={unsent.sending} onChange={choose}>
      {profiles.map(p => (
        <vscode-option key={p.name} value={p.name} description={p.description} selected={p.name === unsent.profile}>
          {[p.harness, p.model, p.effort].join(' · ')}
        </vscode-option>
      ))}
    </vscode-single-select>
  );
}
