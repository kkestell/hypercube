import {useEffect, useState} from 'react';
import {fetchConfig, fetchVersion, saveConfig} from '../api.ts';
import {Button} from '../components/ui/button';
import {Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle} from '../components/ui/dialog';
import {Label} from '../components/ui/label';
import {NativeSelect, NativeSelectOption} from '../components/ui/native-select';
import {useModal} from '../shared/overlays.ts';
import {useStore} from '../store.ts';
import {receivedConfig} from './config.ts';
import {SettingsPermissions} from './settings-permissions.tsx';
import {SettingsProfiles, type ProfileDraft} from './settings-profiles.tsx';

const sections = ['General', 'Agent profiles', 'Permissions', 'About'] as const;
type Section = typeof sections[number];

export function SettingsDialog({open, onOpenChange}: {open: boolean; onOpenChange: (open: boolean) => void}) {
  const focus = useModal(open);
  const [saving, setSaving] = useState(false);
  return <Dialog open={open} onOpenChange={next => { if (!saving) onOpenChange(next); }}>
    <DialogContent {...focus} showCloseButton={!saving} className="flex h-[min(790px,90dvh)] flex-col gap-0 overflow-hidden p-0 sm:max-w-5xl">
      {open && <SettingsContent close={() => onOpenChange(false)} saving={saving} setSaving={setSaving} />}
    </DialogContent>
  </Dialog>;
}

function SettingsContent({close, saving, setSaving}: {close: () => void; saving: boolean; setSaving: (saving: boolean) => void}) {
  const [, store] = useStore();
  const [section, setSection] = useState<Section>('General');
  const [draft, setDraft] = useState<ProfileDraft | null>(null);
  const [loadFailed, setLoadFailed] = useState(false);
  const [error, setError] = useState('');
  const [version, setVersion] = useState('');
  useEffect(() => {
    let active = true;
    fetchConfig(store).then(config => {
      if (!active) return;
      if (config) setDraft({profiles: config.profiles, defaultIndex: config.profiles.findIndex(p => p.name === config.default_profile)});
      else setLoadFailed(true);
    });
    fetchVersion(store).then(result => { if (active && result) setVersion(result); });
    return () => { active = false; };
  }, [store]);
  const save = async () => {
    if (!draft) return;
    setError('');
    setSaving(true);
    try {
      const config = {default_profile: draft.profiles[draft.defaultIndex].name, profiles: draft.profiles};
      await saveConfig(config);
      store.update(state => receivedConfig(state, config));
      close();
    } catch (error) {
      setError(error instanceof Error ? error.message : String(error));
    } finally {
      setSaving(false);
    }
  };
  return <>
    <DialogHeader className="shrink-0 border-b px-6 py-5">
      <DialogTitle>Settings</DialogTitle><DialogDescription>Preferences for Hypercube on this Mac.</DialogDescription>
    </DialogHeader>
    <div className="flex min-h-0 flex-1 flex-col sm:flex-row">
      <nav aria-label="Settings sections" className="flex shrink-0 gap-1 overflow-x-auto border-b p-3 sm:w-44 sm:flex-col sm:border-r sm:border-b-0">
        {sections.map(name => <Button key={name} type="button" className="justify-start" variant={section === name ? 'secondary' : 'ghost'}
          aria-current={section === name ? 'page' : undefined} onClick={() => setSection(name)}>{name}</Button>)}
      </nav>
      <div className="min-w-0 flex-1 overflow-y-auto p-6">
        <h2 className="mb-2 text-xl font-semibold">{section === 'About' ? 'About Hypercube' : section}</h2>
        <div className="mt-5">
          {(section === 'General' || section === 'Agent profiles') && !draft && <p>{loadFailed ? 'Could not load settings. Close and reopen to try again.' : 'Loading settings…'}</p>}
          <fieldset disabled={saving} className="min-w-0">
            {section === 'General' && draft && <section className="grid gap-6">
              <p className="text-sm text-muted-foreground">Choose the agent profile selected when you open a new session.</p>
              <div className="grid gap-3"><Label htmlFor="default-profile">Default agent profile</Label>
                <NativeSelect id="default-profile" value={draft.defaultIndex} onChange={e => setDraft({...draft, defaultIndex: Number(e.target.value)})}>
                  {draft.profiles.map((p, i) => <NativeSelectOption key={i} value={i}>{p.name || 'Untitled profile'}</NativeSelectOption>)}
                </NativeSelect></div>
              <p className="text-sm text-muted-foreground">Existing sessions keep their current model and effort.</p>
            </section>}
            {section === 'Agent profiles' && draft && <SettingsProfiles draft={draft} onChange={setDraft} />}
          </fieldset>
          {section === 'Permissions' && <SettingsPermissions />}
          {section === 'About' && <section aria-label="Build" className="grid gap-3"><h3 className="font-semibold">Build</h3><p className="break-all text-sm text-muted-foreground">{version}</p></section>}
        </div>
      </div>
    </div>
    <footer className="flex shrink-0 flex-wrap items-center justify-between gap-4 border-t px-6 py-4">
      <div className="min-w-0 flex-1"><p className="text-xs text-muted-foreground">Profile edits apply after saving.</p>{error && <p role="alert" className="mt-2 text-sm text-destructive">{error}</p>}</div>
      <div className="flex gap-2"><Button type="button" variant="outline" disabled={saving} onClick={close}>Cancel</Button><Button type="button" disabled={!draft || saving} onClick={save}>{saving ? 'Saving…' : 'Save changes'}</Button></div>
    </footer>
  </>;
}
