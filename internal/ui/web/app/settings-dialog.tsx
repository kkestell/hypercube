import {useEffect, useState} from 'react';
import {fetchConfig, saveConfig} from '../api.ts';
import {Button} from '../components/ui/button';
import {Dialog, DialogContent, DialogHeader, DialogTitle} from '../components/ui/dialog';
import {useDialogFocus} from '../shared/focus.ts';
import {useStore} from '../store.ts';
import {receivedConfig} from './config.ts';
import {SettingsPermissions} from './settings-permissions.tsx';
import {SettingsAgents, SettingsGeneral, SettingsNotifications} from './settings-preferences.tsx';
import {sections, type Config} from './types.ts';

export function SettingsDialog() {
  const [state, store] = useStore();
  const open = state.settings !== null;
  const close = () => store.update(s => ({...s, settings: null}));
  const focus = useDialogFocus(open);
  const [saving, setSaving] = useState(false);
  return <Dialog open={open} onOpenChange={next => { if (!saving && !next) close(); }}>
    <DialogContent {...focus} aria-describedby={undefined} showCloseButton={!saving} className="flex h-[min(790px,90dvh)] flex-col gap-0 overflow-hidden p-0 sm:max-w-5xl">
      {open && <SettingsContent close={close} saving={saving} setSaving={setSaving} />}
    </DialogContent>
  </Dialog>;
}

function SettingsContent({close, saving, setSaving}: {close: () => void; saving: boolean; setSaving: (saving: boolean) => void}) {
  const [state, store] = useStore();
  const section = state.settings;
  const [draft, setDraft] = useState<Config | null>(null);
  const [loadFailed, setLoadFailed] = useState(false);
  const [error, setError] = useState('');
  useEffect(() => {
    let active = true;
    fetchConfig(store).then(config => {
      if (!active) return;
      if (!config) setLoadFailed(true);
      else setDraft(config);
    });
    return () => { active = false; };
  }, [store]);
  const edit = (change: Partial<Config>) => setDraft(draft => draft && {...draft, ...change});
  const save = async () => {
    if (!draft) return;
    setError('');
    setSaving(true);
    try {
      await saveConfig(draft);
      store.update(state => receivedConfig(state, draft));
      close();
    } catch (error) {
      setError(error instanceof Error ? error.message : String(error));
    } finally {
      setSaving(false);
    }
  };
  return <>
    <DialogHeader className="shrink-0 border-b px-6 py-5">
      <DialogTitle>Settings</DialogTitle>
    </DialogHeader>
    <div className="flex min-h-0 flex-1 flex-col sm:flex-row">
      <nav aria-label="Settings sections" className="flex shrink-0 gap-1 overflow-x-auto border-b p-3 sm:w-44 sm:flex-col sm:border-r sm:border-b-0">
        {sections.map(name => <Button key={name} type="button" className="justify-start" variant={section === name ? 'secondary' : 'ghost'}
          aria-current={section === name ? 'page' : undefined} onClick={() => store.update(s => ({...s, settings: name}))}>{name}</Button>)}
      </nav>
      <div className="min-w-0 flex-1 overflow-y-auto p-6">
        <h2 className="mb-2 text-xl font-semibold">{section}</h2>
        <div className="mt-5">
          {section !== 'Permissions' && !draft && <p>{loadFailed ? 'Could not load settings. Close and reopen to try again.' : 'Loading settings…'}</p>}
          <fieldset disabled={saving} className="min-w-0">
            {draft && section === 'General' && <SettingsGeneral draft={draft} onChange={edit} />}
            {draft && section === 'Notifications' && <SettingsNotifications draft={draft} onChange={edit} />}
            {draft && section === 'Agents' && <SettingsAgents draft={draft} onChange={edit} />}
          </fieldset>
          {section === 'Permissions' && <SettingsPermissions />}
        </div>
      </div>
    </div>
    <footer className="flex shrink-0 flex-wrap items-center justify-between gap-4 border-t px-6 py-4">
      <div className="min-w-0 flex-1">{error && <p role="alert" className="text-sm text-destructive">{error}</p>}</div>
      <div className="flex gap-2"><Button type="button" variant="outline" disabled={saving} onClick={close}>Cancel</Button><Button type="button" disabled={!draft || saving} onClick={save}>{saving ? 'Saving…' : 'Save changes'}</Button></div>
    </footer>
  </>;
}
