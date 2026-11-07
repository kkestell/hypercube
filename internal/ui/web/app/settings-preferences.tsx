// The Settings sections that edit preferences: General, Agents, and
// Notifications.
import type {ReactNode} from 'react';
import {ModelSelect} from '../agent/model-select.tsx';
import {Label} from '../components/ui/label';
import {Select, SelectContent, SelectItem, SelectTrigger, SelectValue} from '../components/ui/select';
import {Switch} from '../components/ui/switch';
import {Textarea} from '../components/ui/textarea';
import {Icon} from '../shared/icon.tsx';
import {sessionOf} from '../sessions/selection.ts';
import {useStore} from '../store.ts';
import type {Config, Notifications, Theme} from './types.ts';

interface SectionProps {
  draft: Config;
  onChange: (change: Partial<Config>) => void;
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
      <DefaultSelection draft={draft} onChange={onChange} />
      <Field id="default-location" label="Default location">
        <Choice id="default-location" value={draft.default_worktree ? 'worktree' : 'local'} onChange={value => onChange({default_worktree: value === 'worktree'})}>
          <SelectItem value="local"><Icon name="laptop" />Existing checkout</SelectItem>
          <SelectItem value="worktree"><Icon name="git-branch" />New worktree</SelectItem>
        </Choice>
        <p className="text-sm text-muted-foreground">For Git workspaces. Existing checkout includes uncommitted changes; a new worktree starts from the current commit. Change it for each session below the message box.</p>
      </Field>
    </Group>
  </div>;
}

// DefaultSelection chooses the default settings from what the harnesses offer
// in the shown workspace, or the first.
function DefaultSelection({draft, onChange}: SectionProps) {
  const [state] = useStore();
  const workspace = (state.unsent?.workspace ?? sessionOf(state, state.selected)?.workspace.id) || state.workspaces[0]?.id;
  return <div className="grid gap-3">
    <span className="text-sm font-medium leading-none">Default model</span>
    <ModelSelect value={draft.default_selection ?? null} workspace={workspace ?? null} label="Default model"
      className="self-start" onChange={default_selection => onChange({default_selection})} />
    <p className="text-sm text-muted-foreground">
      {workspace ? 'New sessions start with it until a workspace has sessions; then they start with the settings of its newest session. Existing sessions keep their current model and effort.'
        : 'Add a workspace to choose a default model.'}
    </p>
  </div>;
}

export function SettingsAgents({draft, onChange}: SectionProps) {
  return <div className="grid gap-10">
    <Group title="Child sessions">
      <Field id="child-model-guidance" label="Guidance for choosing child models">
        <Textarea id="child-model-guidance" rows={8} value={draft.child_model_guidance}
          onChange={e => onChange({child_model_guidance: e.target.value})} />
        <p className="text-sm text-muted-foreground">A child session uses its parent's harness, model, and effort unless the user asks for others or this guidance calls for them. Agents follow it as an instruction; it does not limit which models they can choose.</p>
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
      <Toggle id="notify-desktop" label="Show desktop notifications" description="When the Hypercube window is not focused." checked={n.desktop} onChange={desktop => set({desktop})} />
      <Toggle id="notify-sound" label="Play a sound" checked={n.sound} disabled={!n.desktop} onChange={sound => set({sound})} />
    </Group>
  </div>;
}
