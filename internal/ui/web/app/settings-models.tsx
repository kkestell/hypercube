// Model entries apply immediately, separately from the Settings draft.
import {useEffect, useState} from 'react';
import {deleteModelEntry, fetchHarnesses, saveModelEntry} from '../api.ts';
import {harnessIcon} from '../agent/harness-icon.tsx';
import {displayName} from '../agent/selection-label.ts';
import type {Harness, ModelEntry} from '../agent/types.ts';
import {Button} from '../components/ui/button';
import {Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle} from '../components/ui/dialog';
import {Input} from '../components/ui/input';
import {Label} from '../components/ui/label';
import {Select, SelectContent, SelectItem, SelectTrigger, SelectValue} from '../components/ui/select';
import {useStore} from '../store.ts';

const reason = (error: unknown) => error instanceof Error ? error.message : String(error);

export function SettingsModels() {
  const [state, store] = useStore();
  const [editing, setEditing] = useState<ModelEntry | null>(null);
  const [error, setError] = useState('');
  const remove = async (id: string) => {
    setError('');
    try {
      const modelEntries = await deleteModelEntry(id);
      store.update(s => ({...s, modelEntries}));
    } catch (error) { setError(reason(error)); }
  };
  return <section className="grid gap-4">
    <div className="flex items-center justify-between gap-4"><h3 className="text-base font-medium">Models</h3>
      <Button variant="outline" onClick={() => setEditing({id: '', harness: '', model: '', efforts: [], default_effort: ''})}>Add Model</Button></div>
    <p className="text-sm text-muted-foreground">Model changes apply immediately, without Save changes.</p>
    {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
    <div className="divide-y rounded-md border">
      {state.modelEntries?.map(entry => <div key={entry.id} className="flex items-center gap-3 p-3">
        <span title={displayName(entry.harness)}>{harnessIcon(entry.harness) ?? displayName(entry.harness)}</span>
        <div className="min-w-0 flex-1"><p className="truncate text-sm">{entry.model}</p>
          <p className="text-xs text-muted-foreground">{entry.efforts.map(level => level + (entry.efforts.length > 1 && level === entry.default_effort ? ' (default)' : '')).join(' · ')}</p></div>
        <Button variant="outline" size="sm" aria-label={`Edit ${displayName(entry.harness)} ${entry.model}`} onClick={() => setEditing(entry)}>Edit</Button>
        <Button variant="outline" size="sm" aria-label={`Delete ${displayName(entry.harness)} ${entry.model}`} onClick={() => remove(entry.id)}>Delete</Button>
      </div>)}
      {!state.modelEntries?.length && <p className="p-3 text-sm text-muted-foreground">{state.modelEntries === null ? 'Loading models…' : 'No models.'}</p>}
    </div>
    {editing && <EntryDialog entry={editing} close={() => setEditing(null)} />}
  </section>;
}

function EntryDialog({entry, close}: {entry: ModelEntry; close: () => void}) {
  const [, store] = useStore();
  const [harnesses, setHarnesses] = useState<Harness[]>([]);
  const [harness, setHarness] = useState(entry.harness);
  const [model, setModel] = useState(entry.model);
  const [levels, setLevels] = useState(entry.efforts.join(', '));
  const [defaultEffort, setDefaultEffort] = useState(entry.default_effort);
  const [error, setError] = useState('');
  const [saving, setSaving] = useState(false);
  const efforts = levels.trim() ? levels.split(',').map(level => level.trim()) : [];
  useEffect(() => {
    const controller = new AbortController();
    fetchHarnesses(controller.signal).then(setHarnesses, error => {
      if (!controller.signal.aborted) setError(reason(error));
    });
    return () => controller.abort();
  }, []);
  const choices = entry.harness && !harnesses.some(h => h.id === entry.harness) ? [...harnesses, {id: entry.harness}] : harnesses;
  const changeLevels = (text: string) => {
    setLevels(text);
    const next = text.trim() ? text.split(',').map(level => level.trim()) : [];
    if (!next.includes(defaultEffort)) setDefaultEffort(next.at(-1) ?? '');
  };
  const save = async () => {
    setError(''); setSaving(true);
    try {
      const modelEntries = await saveModelEntry({...entry, harness, model, efforts, default_effort: efforts.length === 1 ? efforts[0]! : efforts.length ? defaultEffort : ''});
      store.update(s => ({...s, modelEntries}));
      close();
    } catch (error) { setError(reason(error)); setSaving(false); }
  };
  return <Dialog open onOpenChange={open => { if (!open && !saving) close(); }}>
    <DialogContent aria-describedby={undefined} showCloseButton={!saving}>
      <DialogHeader><DialogTitle>{entry.id ? 'Edit Model' : 'Add Model'}</DialogTitle></DialogHeader>
      <form className="grid gap-4" onSubmit={event => { event.preventDefault(); void save(); }}>
        <fieldset disabled={saving} className="grid gap-4">
          <div className="grid gap-2"><Label htmlFor="model-harness">Harness</Label>
            <Select value={harness} onValueChange={setHarness}><SelectTrigger id="model-harness" className="w-full"><SelectValue placeholder="Choose a harness" /></SelectTrigger>
              <SelectContent>{choices.map(h => <SelectItem key={h.id} value={h.id}>{harnessIcon(h.id)}{h.name || displayName(h.id)}</SelectItem>)}</SelectContent></Select></div>
          <div className="grid gap-2"><Label htmlFor="model-id">Model ID</Label><Input id="model-id" required value={model} onChange={e => setModel(e.target.value)} /></div>
          <div className="grid gap-2"><Label htmlFor="model-efforts">Effort levels</Label><Input id="model-efforts" placeholder="low, medium, high" value={levels} onChange={e => changeLevels(e.target.value)} />
            <p className="text-xs text-muted-foreground">Separate levels with commas, lowest first.</p></div>
          {efforts.length > 1 && <div className="grid gap-2"><Label htmlFor="model-default-effort">Default effort</Label>
            <Select value={defaultEffort} onValueChange={setDefaultEffort}><SelectTrigger id="model-default-effort" className="w-full"><SelectValue /></SelectTrigger>
              <SelectContent>{[...new Set(efforts)].filter(Boolean).map(level => <SelectItem key={level} value={level}>{level}</SelectItem>)}</SelectContent></Select></div>}
        </fieldset>
        {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
        <DialogFooter><Button type="button" variant="outline" disabled={saving} onClick={close}>Cancel</Button><Button type="submit" disabled={saving}>{saving ? 'Saving…' : 'Save'}</Button></DialogFooter>
      </form>
    </DialogContent>
  </Dialog>;
}
