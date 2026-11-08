// Settings as people see them: the harness mark, the model, and the effort
// dimmed so it reads apart from the model.
import {harnessIcon} from './harness-icon.tsx';
import {displayName} from './selection-label.ts';
import type {Settings} from './types.ts';

export function SelectionName({settings}: {settings: Settings}) {
  return <>
    {harnessIcon(settings.harness) ?? displayName(settings.harness)}
    <span className="min-w-0 truncate">{settings.model}</span>
    {settings.effort && <span className="ml-auto shrink-0 opacity-60">{settings.effort}</span>}
  </>;
}
