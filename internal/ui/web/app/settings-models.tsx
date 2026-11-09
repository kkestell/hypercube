// The model picker's entries, edited in the Settings draft.
import {useEffect, useState} from 'react';
import {fetchHarnesses} from '../api.ts';
import {harnessIcon} from '../agent/harness-icon.tsx';
import {displayName} from '../agent/selection-label.ts';
import type {Harness, ModelEntry} from '../agent/types.ts';
import {Button} from '../components/ui/button';
import {Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle} from '../components/ui/dialog';
import {Input} from '../components/ui/input';
import {Label} from '../components/ui/label';
import {Select, SelectContent, SelectItem, SelectTrigger, SelectValue} from '../components/ui/select';

const blank: ModelEntry = {harness: '', model: '', title: '', efforts: [], default_effort: ''};

export function SettingsModels({models, onChange}: {models: ModelEntry[]; onChange: (models: ModelEntry[]) => void}) {
  // index is the edited entry's position in models, or null for a new entry.
  const [editing, setEditing] = useState<{index: number | null; entry: ModelEntry} | null>(null);
  return <section className="grid gap-4">
    <div className="flex items-center justify-between gap-4"><h3 className="text-base font-medium">Models</h3>
      <Button variant="outline" onClick={() => setEditing({index: null, entry: blank})}>Add Model</Button></div>
    <div className="divide-y rounded-md border">
      {models.map((entry, index) => <div key={`${entry.harness}\n${entry.model}`} className="flex items-center gap-3 p-3">
        <span title={displayName(entry.harness)}>{harnessIcon(entry.harness) ?? displayName(entry.harness)}</span>
        <div className="min-w-0 flex-1"><p className="truncate text-sm">{entry.title}</p>
          <p className="truncate text-xs text-muted-foreground">{entry.model}</p>
          <p className="text-xs text-muted-foreground">{entry.efforts.map(level => level + (entry.efforts.length > 1 && level === entry.default_effort ? ' (default)' : '')).join(' · ')}</p></div>
        <Button variant="outline" size="sm" aria-label={`Edit ${displayName(entry.harness)} ${entry.title}`} onClick={() => setEditing({index, entry})}>Edit</Button>
        <Button variant="outline" size="sm" aria-label={`Delete ${displayName(entry.harness)} ${entry.title}`} onClick={() => onChange(models.filter((_, i) => i !== index))}>Delete</Button>
      </div>)}
      {!models.length && <p className="p-3 text-sm text-muted-foreground">No models.</p>}
    </div>
    {editing && <EntryDialog entry={editing.entry} adding={editing.index === null}
      others={models.filter((_, i) => i !== editing.index)} close={() => setEditing(null)}
      save={entry => { onChange(editing.index === null ? [...models, entry] : models.with(editing.index, entry)); setEditing(null); }} />}
  </section>;
}

// EntryDialog edits one entry and checks it against the others in the list
// before handing it to save.
function EntryDialog({entry, adding, others, save, close}: {
  entry: ModelEntry; adding: boolean; others: ModelEntry[]; save: (entry: ModelEntry) => void; close: () => void;
}) {
  const [harnesses, setHarnesses] = useState<Harness[]>([]);
  const [harness, setHarness] = useState(entry.harness);
  const [model, setModel] = useState(entry.model);
  const [title, setTitle] = useState(entry.title);
  const [levels, setLevels] = useState(entry.efforts.join(', '));
  const [chosenDefault, setChosenDefault] = useState(entry.default_effort);
  const [error, setError] = useState('');
  const efforts = levels.split(',').map(level => level.trim()).filter(Boolean);
  // The default follows the last level until the user chooses one still listed.
  const defaultEffort = efforts.includes(chosenDefault) ? chosenDefault : efforts.at(-1) ?? '';
  useEffect(() => {
    const controller = new AbortController();
    fetchHarnesses(controller.signal).then(setHarnesses, error => {
      if (!controller.signal.aborted) setError(error instanceof Error ? error.message : String(error));
    });
    return () => controller.abort();
  }, []);
  const choices = entry.harness && !harnesses.some(h => h.id === entry.harness) ? [...harnesses, {id: entry.harness}] : harnesses;
  const submit = () => {
    const id = model.trim(), name = title.trim();
    const repeated = efforts.find((level, i) => efforts.indexOf(level) !== i);
    const problem = !harness ? 'Choose a harness.'
      : !id ? 'Enter a model ID.'
      : !name ? 'Enter a title.'
      : repeated ? `Effort level "${repeated}" repeats.`
      : others.some(e => e.harness === harness && e.model === id) ? `${displayName(harness)} already lists ${id}.`
      : '';
    if (problem) setError(problem);
    else save({harness, model: id, title: name, efforts, default_effort: defaultEffort});
  };
  return <Dialog open onOpenChange={open => { if (!open) close(); }}>
    <DialogContent aria-describedby={undefined}>
      <DialogHeader><DialogTitle>{adding ? 'Add Model' : 'Edit Model'}</DialogTitle></DialogHeader>
      <form className="grid gap-4" onSubmit={event => { event.preventDefault(); submit(); }}>
        <div className="grid gap-2"><Label htmlFor="model-harness">Harness</Label>
          <Select value={harness} onValueChange={setHarness}><SelectTrigger id="model-harness" className="w-full"><SelectValue placeholder="Choose a harness" /></SelectTrigger>
            <SelectContent>{choices.map(h => <SelectItem key={h.id} value={h.id}>{harnessIcon(h.id)}{h.name || displayName(h.id)}</SelectItem>)}</SelectContent></Select></div>
        <div className="grid gap-2"><Label htmlFor="model-id">Model ID</Label><Input id="model-id" required value={model} onChange={e => setModel(e.target.value)} /></div>
        <div className="grid gap-2"><Label htmlFor="model-title">Title</Label><Input id="model-title" required placeholder="Claude Opus" value={title} onChange={e => setTitle(e.target.value)} /></div>
        <div className="grid gap-2"><Label htmlFor="model-efforts">Effort levels</Label><Input id="model-efforts" placeholder="low, medium, high" value={levels} onChange={e => setLevels(e.target.value)} />
          <p className="text-xs text-muted-foreground">Separate levels with commas, lowest first.</p></div>
        {efforts.length > 1 && <div className="grid gap-2"><Label htmlFor="model-default-effort">Default effort</Label>
          <Select value={defaultEffort} onValueChange={setChosenDefault}><SelectTrigger id="model-default-effort" className="w-full"><SelectValue /></SelectTrigger>
            <SelectContent>{[...new Set(efforts)].map(level => <SelectItem key={level} value={level}>{level}</SelectItem>)}</SelectContent></Select></div>}
        {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
        <DialogFooter><Button type="button" variant="outline" onClick={close}>Cancel</Button><Button type="submit">Save</Button></DialogFooter>
      </form>
    </DialogContent>
  </Dialog>;
}
