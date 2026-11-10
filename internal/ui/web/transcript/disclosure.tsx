// A transcript row that opens to show more. Whether it is open lives in the
// store under the row's key, so it survives the row being drawn again.
import type {ReactNode} from 'react';
import {Collapsible, CollapsibleContent, CollapsibleTrigger} from '../components/ui/collapsible';
import {slide} from '../shared/motion.ts';
import {Chevron} from '../shared/icon.tsx';
import {useStore} from '../store.ts';
import {setExpanded} from './transcripts.ts';

// useExpanded is whether the user opened the row keyed id, and a function to
// open or close it.
export function useExpanded(id: string) {
  const [state, store] = useStore();
  return [!!state.session?.expanded[id], (open: boolean) => store.update(s => setExpanded(s, id, open))] as const;
}

interface DisclosureProps {
  id: string;
  className?: string;
  summary: ReactNode;
  preview?: ReactNode;
  marker?: ReactNode;
  children?: ReactNode;
}

// Open content lines up under the heading text. A disclosure with a marker,
// such as a tool call's status, draws it in a column before the heading, and
// the column's width indents the content; one without draws a chevron after
// the heading. A preview follows the heading while the disclosure is closed.
export function Disclosure({id, className = '', summary, preview, marker, children}: DisclosureProps) {
  const [open, setOpen] = useExpanded(id);
  return <Collapsible className={`[--disclosure-indent:--spacing(6)] ${className}`} open={open} onOpenChange={setOpen}>
    <CollapsibleTrigger className="flex w-full items-start rounded-md py-1 text-left">
      {marker && <span className="flex h-5 w-(--disclosure-indent) shrink-0 items-center">{marker}</span>}
      <span className="flex min-w-0 flex-1 items-center gap-2">{summary}{!open && preview}{!marker && <Chevron open={open} />}</span>
    </CollapsibleTrigger>
    <CollapsibleContent className={`${slide} ${marker ? 'pl-(--disclosure-indent)' : ''}`}>{children}</CollapsibleContent>
  </Collapsible>;
}
