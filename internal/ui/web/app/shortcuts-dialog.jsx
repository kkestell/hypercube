import {Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle} from '../components/ui/dialog';
import {Kbd, KbdGroup} from '../components/ui/kbd';
import {shortcuts} from './shortcuts.js';
import {useModal} from '../shared/overlays.js';

export function ShortcutsDialog({open, onOpenChange}) {
  const focus = useModal(open);
  return <Dialog open={open} onOpenChange={onOpenChange}>
    <DialogContent {...focus} className="max-h-[80vh] overflow-y-auto">
      <DialogHeader><DialogTitle>Keyboard shortcuts</DialogTitle>
        <DialogDescription>Keyboard shortcuts available in Hypercube.</DialogDescription></DialogHeader>
      {['Session', 'View', 'Help'].map(section => <section key={section}>
        <h2 className="mb-2 font-semibold">{section}</h2>
        <dl className="space-y-2">{shortcuts.filter(s => s.section === section && (!s.native || globalThis.webkit?.messageHandlers?.window)).map(s =>
          <div key={s.id} className="flex items-center justify-between gap-4">
            <dt>{s.title}</dt><dd className="flex gap-2">{s.keys.map(keys => <KbdGroup key={keys}>{(keys.match(/[⌘⌥⌃⇧]|[^⌘⌥⌃⇧]+/g) ?? []).map((key, i) => <Kbd key={i}>{key}</Kbd>)}</KbdGroup>)}</dd>
          </div>)}</dl>
      </section>)}
    </DialogContent>
  </Dialog>;
}
