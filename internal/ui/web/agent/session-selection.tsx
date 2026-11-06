// A session's settings below its composer. A new session chooses any
// installed harness; a running session changes its model and effort within
// its harness once it is ready, and shows its settings while it is not.
import {selectModel} from '../api.ts';
import {sessionOf, unsentSettingsChosen} from '../sessions/selection.ts';
import type {Session} from '../sessions/types.ts';
import {Icon} from '../shared/icon.tsx';
import {useStore} from '../store.ts';
import {isUnsent, unsentOf} from './drafts.ts';
import {ModelSelect} from './model-select.tsx';
import {sameSettings, settingsText} from './selection-label.ts';
import type {Settings} from './types.ts';

export function SessionSelection({session}: {session: Session}) {
  const [state, store] = useStore();
  const unsent = isUnsent(session.id) ? unsentOf(state, session.id) : undefined;
  if (unsent) {
    return <ModelSelect value={unsent.settings} place={unsent.workspace ? {workspace: unsent.workspace} : null}
      disabled={unsent.sending} onChange={settings => store.update(s => unsentSettingsChosen(s, settings))} />;
  }
  const parent = sessionOf(state, session.id)?.parent;
  return <span className="flex min-w-0 items-center gap-1">
    <ModelSelect value={session} place={{session: session.id}} fixedHarness disabled={!session.ready || session.ended}
      onChange={settings => selectModel(store, session.id, settings.model, settings.effort)} />
    {parent && <SettingsDiffer child={session} parent={parent} />}
  </span>;
}

// SettingsDiffer marks a child whose settings differ from its parent's.
export function SettingsDiffer({child, parent}: {child: Settings; parent: Settings}) {
  if (sameSettings(child, parent)) return null;
  return <Icon name="differs" role="img" className="text-muted-foreground"
    title={`Settings differ from the parent's: ${settingsText(child)}; parent ${settingsText(parent)}`} />;
}
