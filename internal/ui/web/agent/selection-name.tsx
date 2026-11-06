// Settings as people see them: the harness mark, the model, and the effort
// dimmed so it reads apart from the model.
import {harnessIcon} from './harness-icon.tsx';
import {displayName, modelLabel} from './selection-label.ts';
import type {Model, Settings} from './types.ts';

export function SelectionName({settings, models}: {settings: Settings; models?: Model[]}) {
  return <>
    {harnessIcon(settings.harness) ?? displayName(settings.harness)}
    <span className="min-w-0 truncate">{modelLabel(settings.model, models)}</span>
    {settings.effort && <span className="ml-auto shrink-0 opacity-60">{settings.effort}</span>}
  </>;
}
