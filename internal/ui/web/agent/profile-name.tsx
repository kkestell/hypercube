// A profile as people see it: the harness mark, the model name, and the effort
// dimmed so it reads apart from the name.
import {harnessIcon} from './harness-icon.tsx';
import {displayName, modelName} from './profile-label.ts';
import type {Profile} from './types.ts';

export function ProfileName({profile}: {profile: Pick<Profile, 'harness' | 'model' | 'effort'>}) {
  return <>
    {harnessIcon(profile.harness) ?? displayName(profile.harness)}
    <span className="min-w-0 truncate">{profile.model ? modelName(profile.model) : 'Untitled profile'}</span>
    {profile.effort && <span className="ml-auto shrink-0 opacity-60">{displayName(profile.effort)}</span>}
  </>;
}
