// A session's settings below its composer. A new session chooses any
// installed harness; a running session changes its model and effort within
// its harness once it is ready, and shows its settings while it is not.
import {selectModel} from '../api.ts';
import {Tooltip, TooltipContent, TooltipProvider, TooltipTrigger} from '../components/ui/tooltip';
import {sessionOf, unsentSettingsChosen} from '../sessions/selection.ts';
import type {Session} from '../sessions/types.ts';
import {Icon} from '../shared/icon.tsx';
import {useStore} from '../store.ts';
import {isUnsent, unsentOf} from './drafts.ts';
import {useModels} from './catalogs.ts';
import {ModelSelect} from './model-select.tsx';
import {sameSettings, settingsText} from './selection-label.ts';
import type {Settings} from './types.ts';

export function SessionSelection({session}: {session: Session}) {
  const [state, store] = useStore();
  const unsent = isUnsent(session.id) ? unsentOf(state, session.id) : undefined;
  if (unsent) {
    return <ModelSelect key={unsent.workspace} value={unsent.settings} workspace={unsent.workspace}
      disabled={unsent.sending} onChange={settings => store.update(s => unsentSettingsChosen(s, settings))} />;
  }
  const found = sessionOf(state, session.id);
  const workspace = found?.workspace.id ?? null;
  const parent = found?.parent;
  return <span className="flex min-w-0 items-center gap-1">
    <ModelSelect key={session.id} value={session} workspace={workspace} fixedHarness disabled={!session.ready || session.ended}
      onChange={settings => selectModel(store, session.id, settings.model, settings.effort)} />
    {parent && <SettingsDiffer child={session} parent={parent} workspace={workspace} />}
  </span>;
}

// SettingsDiffer marks a child whose settings differ from its parent's.
export function SettingsDiffer({child, parent, workspace}: {child: Settings; parent: Settings; workspace: string | null}) {
  const childModels = useModels(workspace, child.harness);
  const parentModels = useModels(workspace, parent.harness);
  if (sameSettings(child, parent)) return null;
  const description = `Settings differ from the parent's: ${settingsText(child, childModels)}; parent ${settingsText(parent, parentModels)}`;
  return <TooltipProvider><Tooltip>
    <TooltipTrigger asChild>
      <span role="img" tabIndex={0} aria-label={description} className="inline-flex text-muted-foreground">
        <Icon name="differs" />
      </span>
    </TooltipTrigger>
    <TooltipContent>{description}</TooltipContent>
  </Tooltip></TooltipProvider>;
}
