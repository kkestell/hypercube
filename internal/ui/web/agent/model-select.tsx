// Chooses only from the user's saved model entries, without plugin discovery.
import {useRef, useState} from 'react';
import {openSettings} from '../app/workbench.ts';
import {Button} from '../components/ui/button';
import {Command, CommandEmpty, CommandInput, CommandItem, CommandList} from '../components/ui/command';
import {Popover, PopoverContent, PopoverTrigger} from '../components/ui/popover';
import {Slider} from '../components/ui/slider';
import {Icon} from '../shared/icon.tsx';
import {useStore} from '../store.ts';
import {harnessIcon} from './harness-icon.tsx';
import {displayName, effortLabel, settingsText} from './selection-label.ts';
import {SelectionName} from './selection-name.tsx';
import type {ModelEntry, Settings} from './types.ts';

interface ModelSelectProps {
  value: Settings | null;
  onChange: (settings: Settings) => void;
  fixedHarness?: boolean;
  disabled?: boolean;
  label?: string;
  id?: string;
  className?: string;
  // entries is the list to choose from; the saved list when omitted.
  entries?: ModelEntry[];
}

export function ModelSelect({value, onChange, fixedHarness = false, disabled = false, label = 'Model', id, className = '', entries: given}: ModelSelectProps) {
  const [state, store] = useStore();
  const [open, setOpen] = useState(false);
  const input = useRef<HTMLInputElement>(null);
  const list = given ?? state.config?.models ?? null;
  const entries = list ?? [];
  const models = fixedHarness ? entries.filter(e => e.harness === value?.harness) : entries;
  const current = models.find(e => e.model === value?.model && e.harness === value.harness);
  const empty = !entries.length ? 'No models configured.' : !models.length ? `No ${displayName(value?.harness)} models configured.` : null;
  return <Popover open={open} onOpenChange={setOpen}>
    <PopoverTrigger asChild>
      <Button id={id} type="button" variant="ghost" size="sm" disabled={disabled}
        className={`h-6 min-w-0 max-w-full text-xs font-normal text-muted-foreground ${className}`}
        aria-label={value ? `${label}: ${settingsText(value, entries)}` : label} title={value ? settingsText(value, entries) : undefined}>
        {value ? <SelectionName settings={value} entries={entries} /> : 'Choose a model'}<Icon name="chevron-down" />
      </Button>
    </PopoverTrigger>
    <PopoverContent side="top" align="start" className="flex w-80 max-w-[calc(100vw-2rem)] flex-col gap-2 p-2"
      onOpenAutoFocus={event => { event.preventDefault(); input.current?.focus(); }}>
      {list === null ? <p className="p-2 text-sm">Loading models…</p> : empty ? <div className="p-2 text-sm">
        <p>{empty}</p><Button variant="link" className="px-0" onClick={() => { setOpen(false); store.update(s => openSettings(s, 'Agents')); }}>Open Settings</Button>
      </div> : <>
        <Command loop>
          <CommandInput ref={input} placeholder="Search models" aria-label="Search models" />
          <CommandList className="max-h-64">
            <CommandEmpty>No models match.</CommandEmpty>
            {models.map(entry => {
              const selected = entry === current;
              return <CommandItem key={`${entry.harness}:${entry.model}`} value={`${entry.harness}:${entry.model}`} keywords={[entry.title, displayName(entry.harness)]} aria-selected={selected}
                onSelect={() => { if (!selected) onChange({harness: entry.harness, model: entry.model, effort: entry.default_effort}); }}>
                {harnessIcon(entry.harness) ?? <span className="size-4 shrink-0" />}
                <span className="min-w-0 flex-1 truncate">{entry.title}</span><Icon name="check" className={selected ? '' : 'invisible'} />
              </CommandItem>;
            })}
          </CommandList>
        </Command>
        <Effort key={`${current?.harness}:${current?.model}:${current?.efforts.join(',')}:${value?.effort}`} entry={current} value={value} onChange={onChange} />
      </>}
    </PopoverContent>
  </Popover>;
}

// Effort is always shown; it only moves when the selected model offers a choice.
function Effort({entry, value, onChange}: {entry?: ModelEntry; value: Settings | null; onChange: (settings: Settings) => void}) {
  const [dragged, setDragged] = useState<number | null>(null);
  const efforts = entry?.efforts ?? [];
  const index = efforts.indexOf(value?.effort ?? '');
  const position = dragged ?? (index < 0 ? Math.max(efforts.indexOf(entry?.default_effort ?? ''), 0) : index);
  const name = dragged === null ? value?.effort || 'Default' : efforts[position];
  return <div className="grid gap-3 p-2">
    <span className="text-sm">Effort</span>
    <Slider min={0} max={Math.max(efforts.length - 1, 1)} step={1} value={[position]} disabled={!entry || !value || efforts.length < 2}
      thumbProps={{'aria-label': 'Effort', 'aria-valuetext': name}}
      onValueChange={([next]) => setDragged(next!)} onValueCommit={([next]) => { setDragged(null); onChange({...value!, effort: efforts[next!]!}); }} />
    {efforts.length > 0 && <Ticks efforts={efforts} />}
  </div>;
}

// Ticks sit where the slider thumb's centre lands for each step; the thumb stays inside the track, so its centre is offset by 8px at the left end and -8px at the right.
function Ticks({efforts}: {efforts: string[]}) {
  const last = Math.max(efforts.length - 1, 1);
  return <div className="relative -mt-1 h-9 text-xs text-muted-foreground" aria-hidden>
    {efforts.map((effort, i) => <span key={effort} className="absolute top-0 flex -translate-x-1/2 flex-col items-center gap-1"
      style={{left: `calc(${(i / last) * 100}% + ${8 - (i / last) * 16}px)`}}>
      <span className="h-1.5 w-px bg-muted-foreground" />{effortLabel(effort)}
    </span>)}
  </div>;
}
