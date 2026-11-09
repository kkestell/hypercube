// Settings as people see them: the harness mark, the model, and the effort
// dimmed so it reads apart from the model.
import {harnessIcon} from './harness-icon.tsx';
import {useStore} from '../store.ts';
import {displayName, modelTitle} from './selection-label.ts';
import type {ModelEntry, Settings} from './types.ts';

// entries supplies the titles; the saved list when omitted.
export function SelectionName({settings, entries}: {settings: Settings; entries?: ModelEntry[] | null}) {
  const [state] = useStore();
  return <>
    {harnessIcon(settings.harness) ?? displayName(settings.harness)}
    <span className="min-w-0 truncate">{modelTitle(settings, entries ?? state.config?.models ?? null)}</span>
    {settings.effort && <span className="ml-auto shrink-0 opacity-60">{settings.effort}</span>}
  </>;
}
