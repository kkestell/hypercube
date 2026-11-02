// The workspace's profile choices, with settings above each description.
import {useLayoutEffect, useRef} from 'preact/hooks';
import {usePopup} from '../shared/popup.js';
import './profile-picker.css';

export function ProfilePicker({x, y, profiles, onChoose, onClose}) {
  const ref = useRef(null);
  usePopup(ref, x, y, onClose);
  useLayoutEffect(() => ref.current.querySelector('button')?.focus(), []);

  const choose = profile => {
    onClose();
    onChoose(profile);
  };
  const key = (e, index) => {
    if (e.key === 'Enter') {
      e.preventDefault();
      choose(profiles[index]);
    } else if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
      e.preventDefault();
      const next = (index + (e.key === 'ArrowDown' ? 1 : -1) + profiles.length) % profiles.length;
      ref.current.children[next].focus();
    }
  };

  return (
    <div ref={ref} class="profile-picker" aria-label="Session profiles">
      {profiles.map((profile, index) => (
        <button key={profile.name} type="button" title={profile.description} onClick={() => choose(profile)} onKeyDown={e => key(e, index)}>
          <span class="profile-picker-settings">{[profile.harness, profile.model, profile.effort].join(' · ')}</span>
          <span class="profile-picker-description">{profile.description}</span>
        </button>
      ))}
    </div>
  );
}
