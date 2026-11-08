// Chooses only from the user's saved model entries, without plugin discovery.
import {useRef, useState} from 'react';
import {Button} from '../components/ui/button';
import {Command, CommandEmpty, CommandInput, CommandItem, CommandList} from '../components/ui/command';
import {Popover, PopoverContent, PopoverTrigger} from '../components/ui/popover';
import {Slider} from '../components/ui/slider';
import {Tabs, TabsList, TabsTrigger} from '../components/ui/tabs';
import {Icon} from '../shared/icon.tsx';
import {useStore} from '../store.ts';
import {harnessIcon} from './harness-icon.tsx';
import {displayName, settingsText} from './selection-label.ts';
import {SelectionName} from './selection-name.tsx';
import type {ModelEntry, Settings} from './types.ts';

interface ModelSelectProps {
  value: Settings | null;
  onChange: (settings: Settings) => void;
  fixedHarness?: boolean;
  disabled?: boolean;
  label?: string;
  className?: string;
}

export function ModelSelect({value, onChange, fixedHarness = false, disabled = false, label = 'Model', className = ''}: ModelSelectProps) {
  const [state, store] = useStore();
  const [open, setOpen] = useState(false);
  const [browsed, setBrowsed] = useState<string | null>(null);
  const input = useRef<HTMLInputElement>(null);
  const entries = state.modelEntries ?? [];
  const harnesses = [...new Set(entries.map(e => e.harness))];
  const harness = fixedHarness ? value?.harness : browsed && harnesses.includes(browsed) ? browsed
    : harnesses.includes(value?.harness ?? '') ? value?.harness : harnesses[0];
  const models = entries.filter(e => e.harness === harness);
  const current = models.find(e => e.model === value?.model && e.harness === value.harness);
  const empty = !entries.length ? 'No models configured.' : fixedHarness && !models.length ? `No ${displayName(harness)} models configured.` : null;
  return <Popover open={open} onOpenChange={next => { setOpen(next); if (!next) setBrowsed(null); }}>
    <PopoverTrigger asChild>
      <Button type="button" variant="ghost" size="sm" disabled={disabled}
        className={`h-6 min-w-0 max-w-full text-xs font-normal text-muted-foreground ${className}`}
        aria-label={value ? `${label}: ${settingsText(value)}` : label} title={value ? settingsText(value) : undefined}>
        {value ? <SelectionName settings={value} /> : 'Choose a model'}<Icon name="chevron-down" />
      </Button>
    </PopoverTrigger>
    <PopoverContent side="top" align="start" className="flex w-80 max-w-[calc(100vw-2rem)] flex-col gap-2 p-2"
      onOpenAutoFocus={event => { event.preventDefault(); input.current?.focus(); }}>
      {!fixedHarness && harnesses.length > 0 && <Tabs value={harness} onValueChange={setBrowsed}>
        <TabsList aria-label="Harness">{harnesses.map(id => <TabsTrigger key={id} value={id} aria-label={displayName(id)} title={displayName(id)}>
          {harnessIcon(id) ?? displayName(id)}
        </TabsTrigger>)}</TabsList>
      </Tabs>}
      {state.modelEntries === null ? <p className="p-2 text-sm">Loading models…</p> : empty ? <div className="p-2 text-sm">
        <p>{empty}</p><Button variant="link" className="px-0" onClick={() => { setOpen(false); store.update(s => ({...s, settings: 'Agents'})); }}>Open Settings</Button>
      </div> : <>
        <Command key={harness} loop>
          <CommandInput ref={input} placeholder="Search models" aria-label="Search models" />
          <CommandList className="max-h-64">
            <CommandEmpty>No models match.</CommandEmpty>
            {models.map(entry => <CommandItem key={entry.id} value={entry.model} aria-selected={current?.id === entry.id}
              onSelect={() => { if (current?.id !== entry.id) onChange({harness: entry.harness, model: entry.model, effort: entry.default_effort}); }}>
              <span className="min-w-0 flex-1 truncate">{entry.model}</span><Icon name="check" className={current?.id === entry.id ? '' : 'invisible'} />
            </CommandItem>)}
          </CommandList>
        </Command>
        {current && value && current.efforts.length > 0 && <Effort key={`${current.id}:${current.efforts.join(',')}:${value.effort}`} entry={current} value={value} onChange={onChange} />}
      </>}
    </PopoverContent>
  </Popover>;
}

function Effort({entry, value, onChange}: {entry: ModelEntry; value: Settings; onChange: (settings: Settings) => void}) {
  const [dragged, setDragged] = useState<number | null>(null);
  const index = entry.efforts.indexOf(value.effort);
  const position = dragged ?? (index < 0 ? entry.efforts.indexOf(entry.default_effort) : index);
  const name = dragged === null ? value.effort || 'Default' : entry.efforts[position];
  return <div className="grid gap-3 p-2">
    <div className="flex justify-between text-sm"><span>Effort</span><span>{entry.efforts.length === 1 ? entry.efforts[0] : name}</span></div>
    {entry.efforts.length > 1 && <Slider min={0} max={entry.efforts.length - 1} step={1} value={[position]}
      thumbProps={{'aria-label': 'Effort', 'aria-valuetext': name}}
      onValueChange={([next]) => setDragged(next!)} onValueCommit={([next]) => { setDragged(null); onChange({...value, effort: entry.efforts[next!]!}); }} />}
  </div>;
}
