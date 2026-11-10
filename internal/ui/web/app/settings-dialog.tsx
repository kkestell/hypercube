// Settings: a section list beside the shown section, which edits a draft of
// the configuration until Save. The draft is loaded fresh each time the
// dialog opens, and the dialog keeps its content while it closes.
import {useState} from 'react';
import {fetchConfig, saveConfig} from '../api.ts';
import {Button} from '../components/ui/button';
import {Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle} from '../components/ui/dialog';
import {errorText} from '../shared/error-text.ts';
import {useDialogFocus} from '../shared/focus.ts';
import {Message} from '../shared/message.tsx';
import {useRequest} from '../shared/use-request.ts';
import {useStore} from '../store.ts';
import {receivedConfig} from './config.ts';
import {SettingsPermissions} from './settings-permissions.tsx';
import {SettingsAgents, SettingsGeneral, SettingsNotifications, SettingsReview} from './settings-preferences.tsx';
import {sections, type Config} from './types.ts';
import {closeDialog, showSettingsSection} from './workbench.ts';

export function SettingsDialog() {
  const [state, store] = useStore();
  const open = state.dialog === 'settings';
  const close = () => store.update(closeDialog);
  const focus = useDialogFocus(open);
  const [saving, setSaving] = useState(false);
  return <Dialog open={open} onOpenChange={next => { if (!saving && !next) close(); }}>
    <DialogContent {...focus} aria-describedby={undefined} showCloseButton={!saving} className="flex h-[min(790px,90dvh)] flex-col gap-0 overflow-hidden p-0 sm:max-w-5xl">
      <SettingsContent close={close} saving={saving} setSaving={setSaving} />
    </DialogContent>
  </Dialog>;
}

// SettingsContent mounts with the dialog's content, so each opening loads
// the configuration again.
function SettingsContent({close, saving, setSaving}: {close: () => void; saving: boolean; setSaving: (saving: boolean) => void}) {
  const [state, store] = useStore();
  const section = state.settingsSection;
  const loaded = useRequest(() => fetchConfig(store), [store]);
  const [edits, setEdits] = useState<Partial<Config>>({});
  const [error, setError] = useState('');
  const draft = loaded.data ? {...loaded.data, ...edits} : null;
  const edit = (change: Partial<Config>) => setEdits(edits => ({...edits, ...change}));
  const save = async () => {
    if (!draft) return;
    setError('');
    setSaving(true);
    try {
      await saveConfig(draft);
      store.update(state => receivedConfig(state, draft));
      close();
    } catch (error) {
      setError(errorText(error));
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
          aria-current={section === name ? 'page' : undefined} onClick={() => store.update(s => showSettingsSection(s, name))}>{name}</Button>)}
      </nav>
      <div className="min-w-0 flex-1 overflow-y-auto p-6">
        <h2 className="mb-5 text-lg font-semibold">{section}</h2>
        {section !== 'Permissions' && !draft && <Message error={loaded.data === null}>{loaded.data === null ? 'Could not load settings. Close and reopen to try again.' : 'Loading settings…'}</Message>}
        <fieldset disabled={saving} className="min-w-0">
          {draft && section === 'General' && <SettingsGeneral draft={draft} onChange={edit} />}
          {draft && section === 'Notifications' && <SettingsNotifications draft={draft} onChange={edit} />}
          {draft && section === 'Agents' && <SettingsAgents draft={draft} onChange={edit} />}
          {draft && section === 'Review' && <SettingsReview draft={draft} onChange={edit} />}
        </fieldset>
        {section === 'Permissions' && <SettingsPermissions />}
      </div>
    </div>
    <DialogFooter className="shrink-0 items-center border-t px-6 py-4">
      {error && <Message error className="mr-auto">{error}</Message>}
      <Button type="button" variant="outline" disabled={saving} onClick={close}>Cancel</Button>
      <Button type="button" disabled={!draft || saving} onClick={save}>{saving ? 'Saving…' : 'Save changes'}</Button>
    </DialogFooter>
  </>;
}
