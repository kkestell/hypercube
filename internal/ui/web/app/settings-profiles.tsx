import {useState} from 'react';
import type {Profile} from '../agent/types.ts';
import {Button} from '../components/ui/button';
import {Input} from '../components/ui/input';
import {Label} from '../components/ui/label';
import {Select, SelectContent, SelectItem, SelectTrigger, SelectValue} from '../components/ui/select';
import {harnessIcon} from '../agent/harness-icon.tsx';
import {ProfileName} from '../agent/profile-name.tsx';
import {Textarea} from '../components/ui/textarea';

const harnesses = [['claude', 'Claude Code'], ['codex', 'Codex'], ['opencode', 'OpenCode'], ['pi', 'Pi']];

// Track the default by position while editing so duplicate or unfinished names
// cannot make it refer to the wrong profile. Save writes its final name.
export interface ProfileDraft {
  profiles: Profile[];
  defaultIndex: number;
}

export function SettingsProfiles({draft, onChange}: {draft: ProfileDraft; onChange: (draft: ProfileDraft) => void}) {
  const [selected, setSelected] = useState(0);
  const {profiles, defaultIndex} = draft;
  const profile = profiles[selected];
  const edit = (change: Partial<Profile>) => onChange({...draft, profiles: profiles.map((p, i) => i === selected ? {...p, ...change} : p)});
  const add = () => {
    onChange({...draft, profiles: [...profiles, {name: '', description: '', harness: 'codex', model: '', effort: ''}]});
    setSelected(profiles.length);
  };
  const remove = () => {
    onChange({profiles: profiles.filter((_, i) => i !== selected),
      defaultIndex: defaultIndex === selected ? 0 : defaultIndex > selected ? defaultIndex - 1 : defaultIndex});
    setSelected(Math.max(0, selected - 1));
  };
  return <div className="flex flex-col gap-6">
    <p className="text-sm text-muted-foreground">Profiles are available in the composer and to agents choosing how to delegate work.</p>
    <div className="grid gap-6 md:grid-cols-[minmax(0,1fr)_minmax(0,1.5fr)]">
      <div className="flex min-w-0 flex-col gap-3">
        <div aria-label="Agent profiles" className="flex max-h-80 flex-col gap-1 overflow-y-auto">
          {profiles.map((p, i) => <Button key={i} type="button" variant={i === selected ? 'secondary' : 'ghost'}
            className="justify-start" aria-pressed={i === selected} onClick={() => setSelected(i)}>
            <ProfileName profile={p} />{i === defaultIndex && <span className="sr-only"> (default)</span>}
          </Button>)}
        </div>
        <Button type="button" variant="outline" onClick={add}>Add profile</Button>
      </div>
      <fieldset key={selected} className="flex min-w-0 flex-col gap-4">
        <legend className="sr-only">Edit {profile.name || 'new profile'}</legend>
        <div className="grid gap-2"><Label htmlFor="profile-name">Profile name</Label>
          <Input autoFocus id="profile-name" value={profile.name} onChange={e => edit({name: e.target.value})} />
          <p className="text-xs text-muted-foreground">A unique name used in the selector and by agents.</p></div>
        <div className="grid gap-2"><Label htmlFor="profile-harness">Harness</Label>
          <Select value={profile.harness} onValueChange={harness => edit({harness})}>
            <SelectTrigger id="profile-harness" className="w-full"><SelectValue /></SelectTrigger>
            <SelectContent>
              {!harnesses.some(([value]) => value === profile.harness) && <SelectItem value={profile.harness}>{profile.harness}</SelectItem>}
              {harnesses.map(([value, label]) => <SelectItem key={value} value={value}>{harnessIcon(value)}{label}</SelectItem>)}
            </SelectContent>
          </Select></div>
        <div className="grid grid-cols-2 gap-3">
          <div className="grid gap-2"><Label htmlFor="profile-model">Model</Label><Input id="profile-model" value={profile.model} onChange={e => edit({model: e.target.value})} /></div>
          <div className="grid gap-2"><Label htmlFor="profile-effort">Effort</Label><Input id="profile-effort" value={profile.effort} onChange={e => edit({effort: e.target.value})} /></div>
        </div>
        <p className="text-xs text-muted-foreground">Use model IDs and effort values accepted by this harness.</p>
        <div className="grid gap-2"><Label htmlFor="profile-description">When to use this profile</Label>
          <Textarea id="profile-description" rows={5} value={profile.description} onChange={e => edit({description: e.target.value})} />
          <p className="text-xs text-muted-foreground">Agents use this description when choosing a profile for delegated work.</p></div>
        <Button type="button" variant="outline" className="self-start" disabled={profiles.length === 1} onClick={remove}>Remove profile</Button>
        {profiles.length === 1 && <p className="text-xs text-muted-foreground">Keep at least one profile.</p>}
      </fieldset>
    </div>
  </div>;
}
