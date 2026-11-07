import {Fragment} from 'react';
import {Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle} from '../components/ui/dialog';
import {Kbd, KbdGroup} from '../components/ui/kbd';
import {keyNames, keySeparator} from './keys.ts';
import {keysOf, shortcuts, type Section} from './shortcuts.ts';
import {hypercube} from './window.ts';
import {useDialogFocus} from '../shared/focus.ts';

const sections: Section[] = ['App', 'Session', 'View', 'Help'];

// Keycaps shows a binding as one key per part, with the platform's separator
// between the parts.
function Keycaps({keys}: {keys: string}) {
  const separator = keySeparator();
  return <KbdGroup>{keyNames(keys).map((key, i) => <Fragment key={i}>
    {i > 0 && separator && <span>{separator}</span>}<Kbd>{key}</Kbd>
  </Fragment>)}</KbdGroup>;
}

export function ShortcutsDialog({open, onOpenChange}: {open: boolean; onOpenChange: (open: boolean) => void}) {
  const focus = useDialogFocus(open);
  return <Dialog open={open} onOpenChange={onOpenChange}>
    <DialogContent {...focus} className="max-h-[80vh] overflow-y-auto">
      <DialogHeader><DialogTitle>Keyboard shortcuts</DialogTitle>
        <DialogDescription>Keyboard shortcuts available in Hypercube.</DialogDescription></DialogHeader>
      {sections.map(section => <section key={section}>
        <h2 className="mb-2 font-semibold">{section}</h2>
        <dl className="space-y-2">{shortcuts.filter(s => s.section === section && (!s.native || hypercube())).map(s =>
          <div key={s.id} className="flex items-center justify-between gap-4">
            <dt>{s.title}</dt><dd className="flex gap-2">{keysOf(s).map(keys => <Keycaps key={keys} keys={keys} />)}</dd>
          </div>)}</dl>
      </section>)}
    </DialogContent>
  </Dialog>;
}
