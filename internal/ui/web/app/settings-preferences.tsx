// The Settings sections that edit preferences: General and Notifications.
import type {ReactNode} from 'react';
import {ProfileName} from '../agent/profile-name.tsx';
import {Label} from '../components/ui/label';
import {Select, SelectContent, SelectItem, SelectTrigger, SelectValue} from '../components/ui/select';
import {Switch} from '../components/ui/switch';
import {Icon} from '../shared/icon.tsx';
import type {ProfileDraft} from './settings-profiles.tsx';
import type {Config, Notifications, Theme} from './types.ts';

// SettingsDraft is the edited config, with the default profile tracked by
// position as in ProfileDraft.
export type SettingsDraft = Omit<Config, 'default_profile'> & ProfileDraft;

interface SectionProps {
  draft: SettingsDraft;
  onChange: (change: Partial<SettingsDraft>) => void;
}

const themes: [Theme, string][] = [['system', 'System'], ['light', 'Light'], ['dark', 'Dark']];
const terminalFontSizes = [12, 13, 14, 16, 18];

function Field({id, label, children}: {id: string; label: string; children: ReactNode}) {
  return <div className="grid gap-3"><Label htmlFor={id}>{label}</Label>{children}</div>;
}

function Choice({id, value, onChange, children}: {id: string; value: string; onChange: (value: string) => void; children: ReactNode}) {
  return <Select value={value} onValueChange={onChange}>
    <SelectTrigger id={id} className="w-full"><SelectValue /></SelectTrigger>
    <SelectContent>{children}</SelectContent>
  </Select>;
}

function Group({title, children}: {title: string; children: ReactNode}) {
  return <section className="grid gap-6"><h3 className="text-base font-medium">{title}</h3>{children}</section>;
}

export function SettingsGeneral({draft, onChange}: SectionProps) {
  return <div className="grid gap-10">
    <Group title="Appearance">
      <Field id="theme" label="Theme">
        <Choice id="theme" value={draft.theme} onChange={value => onChange({theme: value as Theme})}>
          {themes.map(([value, label]) => <SelectItem key={value} value={value}>{label}</SelectItem>)}
        </Choice>
      </Field>
      <Field id="terminal-font-size" label="Terminal text size">
        <Choice id="terminal-font-size" value={String(draft.terminal_font_size)} onChange={value => onChange({terminal_font_size: Number(value)})}>
          {terminalFontSizes.map(size => <SelectItem key={size} value={String(size)}>{size} px</SelectItem>)}
        </Choice>
      </Field>
    </Group>
    <Group title="New sessions">
      <Field id="default-profile" label="Default agent profile">
        <Choice id="default-profile" value={String(draft.defaultIndex)} onChange={value => onChange({defaultIndex: Number(value)})}>
          {draft.profiles.map((p, i) => <SelectItem key={i} value={String(i)}><ProfileName profile={p} /></SelectItem>)}
        </Choice>
        <p className="text-sm text-muted-foreground">Existing sessions keep their current model and effort.</p>
      </Field>
      <Field id="default-location" label="Default location">
        <Choice id="default-location" value={draft.default_worktree ? 'worktree' : 'local'} onChange={value => onChange({default_worktree: value === 'worktree'})}>
          <SelectItem value="local"><Icon name="laptop" />Local</SelectItem>
          <SelectItem value="worktree"><Icon name="git-branch" />Worktree</SelectItem>
        </Choice>
        <p className="text-sm text-muted-foreground">For Git workspaces. The composer can change it for each session.</p>
      </Field>
    </Group>
  </div>;
}

function Toggle({id, label, description, checked, disabled, onChange}: {
  id: string; label: string; description?: string; checked: boolean; disabled?: boolean; onChange: (checked: boolean) => void;
}) {
  return <div className="flex items-center justify-between gap-4">
    <div className="grid gap-1">
      <Label htmlFor={id}>{label}</Label>
      {description && <p className="text-sm text-muted-foreground">{description}</p>}
    </div>
    <Switch id={id} checked={checked} disabled={disabled} onCheckedChange={onChange} />
  </div>;
}

export function SettingsNotifications({draft, onChange}: SectionProps) {
  const n = draft.notifications;
  const set = (change: Partial<Notifications>) => onChange({notifications: {...n, ...change}});
  return <div className="grid gap-10">
    <Group title="Notify me when">
      <Toggle id="notify-requests" label="An agent needs my attention" description="Questions and tool approval requests." checked={n.requests} onChange={requests => set({requests})} />
      <Toggle id="notify-replies" label="An agent has a new reply" checked={n.replies} onChange={replies => set({replies})} />
    </Group>
    <Group title="Delivery">
      <Toggle id="notify-macos" label="Show macOS notifications" description="When the Hypercube window is not focused." checked={n.macos} onChange={macos => set({macos})} />
      <Toggle id="notify-sound" label="Play a sound" checked={n.sound} disabled={!n.macos} onChange={sound => set({sound})} />
    </Group>
  </div>;
}
