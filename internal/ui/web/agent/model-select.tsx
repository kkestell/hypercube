// Chooses a harness, model, and effort from what the harnesses offer in a
// directory. The choices load when the picker opens. Choosing a model loads its
// efforts before the complete settings are committed; a running session's
// harness is fixed.
import {useEffect, useState} from 'react';
import {fetchHarnesses, fetchModels, type ModelsPlace} from '../api.ts';
import {Button} from '../components/ui/button';
import {Command, CommandEmpty, CommandInput, CommandItem, CommandList} from '../components/ui/command';
import {Popover, PopoverContent, PopoverTrigger} from '../components/ui/popover';
import {Select, SelectContent, SelectItem, SelectTrigger, SelectValue} from '../components/ui/select';
import {Icon} from '../shared/icon.tsx';
import {displayName, settingsText} from './selection-label.ts';
import {SelectionName} from './selection-name.tsx';
import type {Catalog, Harness, Settings} from './types.ts';

interface ModelSelectProps {
  value: Settings | null;
  // Where the choices are offered; without one there is nothing to choose.
  place: ModelsPlace | null;
  onChange: (settings: Settings) => void;
  fixedHarness?: boolean;
  disabled?: boolean;
  label?: string;
  className?: string;
}

// The select item that stands for an empty effort.
const harnessDefault = 'harness-default';

const reason = (error: unknown) => error instanceof Error ? error.message : String(error);
const aborted = (error: unknown) => error instanceof DOMException && error.name === 'AbortError';

// Loaded is a request's result, or why it failed, for the key it was made for.
type Loaded<T> = {key: string; value?: T; error?: string};

// useLoad runs load for key while key is set, cancelling superseded requests.
function useLoad<T>(key: string | null, load: (signal: AbortSignal) => Promise<T>): Loaded<T> | null {
  const [loaded, setLoaded] = useState<Loaded<T> | null>(null);
  useEffect(() => {
    if (key === null) return;
    const controller = new AbortController();
    load(controller.signal).then(
      value => setLoaded({key, value}),
      error => { if (!aborted(error)) setLoaded({key, error: reason(error)}); });
    return () => controller.abort();
  }, [key]);
  return loaded && loaded.key === key ? loaded : null;
}

export function ModelSelect({value, place, onChange, fixedHarness = false, disabled = false, label = 'Model', className = ''}: ModelSelectProps) {
  const [open, setOpen] = useState(false);
  const [browsed, setBrowsed] = useState<string | null>(null);
  // The model chosen while its efforts load, and why the last choice failed.
  const [pending, setPending] = useState<string | null>(null);
  const [failure, setFailure] = useState<string | null>(null);
  const placeKey = place && JSON.stringify(place);

  const harnesses = useLoad<Harness[]>(open && !fixedHarness ? 'harnesses' : null, fetchHarnesses);
  const installed = harnesses?.value?.filter(h => h.installed) ?? [];
  const harness = browsed ?? value?.harness ?? installed[0]?.id ?? '';
  const catalog = useLoad<Catalog>(open && place && harness ? `${placeKey}\n${harness}` : null,
    signal => fetchModels(place!, harness, '', signal));
  const effortModel = pending ?? (value?.harness === harness ? value.model : null);
  const efforts = useLoad<Catalog>(open && place && harness && effortModel ? `${placeKey}\n${harness}\n${effortModel}` : null,
    signal => fetchModels(place!, harness, effortModel!, signal));

  // A chosen model is committed with its default effort once its efforts load,
  // or with the current effort when it is the current model.
  useEffect(() => {
    if (!pending || !efforts || efforts.key.split('\n')[2] !== pending) return;
    setPending(null);
    if (efforts.error) {
      setFailure(`Could not choose ${pending}: ${efforts.error}`);
      return;
    }
    const same = value?.harness === harness && value.model === pending;
    const effort = same && (value.effort === '' || efforts.value?.efforts?.includes(value.effort)) ? value.effort : efforts.value?.default_effort ?? '';
    onChange({harness, model: pending, effort});
  }, [efforts, pending]);

  const choose = (model: string) => {
    setFailure(null);
    setPending(model);
  };
  const chooseEffort = (effort: string) => {
    if (!value) return;
    onChange({...value, effort: effort === harnessDefault ? '' : effort});
  };
  const toggle = (next: boolean) => {
    setOpen(next);
    if (!next) {
      setBrowsed(null);
      setPending(null);
      setFailure(null);
    }
  };

  const models = catalog?.value?.models ?? [];
  const shownModels = value?.harness === harness ? models : [];
  const effortChoices = !pending && value?.harness === harness ? efforts?.value?.efforts ?? [] : [];
  const browsedHarness = harnesses?.value?.find(h => h.id === harness);
  const status = !place ? 'Add a workspace to choose a model.'
    : harnesses?.error ?? (harnesses?.value && !installed.length ? 'No harness is installed.' : null)
    ?? (browsedHarness?.error || (browsedHarness && !browsedHarness.installed ? `${browsedHarness.name || displayName(harness)} is not installed.` : null))
    ?? catalog?.error ?? failure ?? (!pending && efforts?.error ? `Could not list the efforts of ${effortModel}: ${efforts.error}` : null) ?? null;

  return (
    <Popover open={open} onOpenChange={toggle}>
      <PopoverTrigger asChild>
        <Button type="button" variant="ghost" size="sm" disabled={disabled}
          className={`h-6 min-w-0 max-w-full text-xs font-normal text-muted-foreground ${className}`}
          aria-label={value ? `${label}: ${settingsText(value, shownModels)}` : label} title={value ? settingsText(value, shownModels) : undefined}>
          {value ? <SelectionName settings={value} models={shownModels} /> : 'Choose a model'}<Icon name="chevron-down" />
        </Button>
      </PopoverTrigger>
      <PopoverContent side="top" align="start" className="flex w-80 max-w-[calc(100vw-2rem)] flex-col gap-2 p-2">
        {!fixedHarness && installed.length > 0 && (
          <Select value={harness} onValueChange={id => { setBrowsed(id); setPending(null); setFailure(null); }}>
            <SelectTrigger aria-label="Harness" className="w-full"><SelectValue /></SelectTrigger>
            <SelectContent>
              {installed.map(h => <SelectItem key={h.id} value={h.id}>{h.name || displayName(h.id)}</SelectItem>)}
            </SelectContent>
          </Select>
        )}
        {status && <p role="alert" className="px-1 text-xs text-destructive">{status}</p>}
        {place && harness && !catalog?.error && (
          <Command className="rounded-md border" loop>
            <CommandInput placeholder="Search models" aria-label="Search models" />
            <CommandList className="max-h-64">
              {!catalog ? <p className="py-6 text-center text-sm text-muted-foreground">Loading models…</p>
                : <CommandEmpty>No models match.</CommandEmpty>}
              {models.map(m => {
                const current = value?.harness === harness && value.model === m.id;
                return (
                  <CommandItem key={m.id} value={m.id} keywords={[m.label ?? '', m.provider ?? '', m.resolved ?? '']} onSelect={() => choose(m.id)}
                    aria-selected={current} disabled={pending !== null}>
                    <span className="flex min-w-0 flex-1 flex-col">
                      <span className="truncate">{m.label || m.id}{m.id === catalog?.value?.default && <span className="text-muted-foreground"> (default)</span>}</span>
                      <span className="truncate text-xs text-muted-foreground">{[m.provider, m.id].filter(Boolean).join(' · ')}</span>
                    </span>
                    {pending === m.id ? <span className="text-xs text-muted-foreground">Loading…</span>
                      : <Icon name="check" className={current ? '' : 'invisible'} />}
                  </CommandItem>
                );
              })}
            </CommandList>
          </Command>
        )}
        {value && effortChoices.length > 0 && (
          <Select value={value.effort || harnessDefault} onValueChange={chooseEffort}>
            <SelectTrigger aria-label="Effort" className="w-full"><SelectValue /></SelectTrigger>
            <SelectContent>
              <SelectItem value={harnessDefault}>Harness default</SelectItem>
              {value.effort && !effortChoices.includes(value.effort) && <SelectItem value={value.effort}>{value.effort}</SelectItem>}
              {effortChoices.map(e => <SelectItem key={e} value={e}>{e}{e === efforts?.value?.default_effort ? ' (default)' : ''}</SelectItem>)}
            </SelectContent>
          </Select>
        )}
      </PopoverContent>
    </Popover>
  );
}
