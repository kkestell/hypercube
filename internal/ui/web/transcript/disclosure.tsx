import {Button} from '../components/ui/button';
import {Collapsible, CollapsibleContent, CollapsibleTrigger} from '../components/ui/collapsible';
import {slide} from '../shared/motion.ts';
import type {ReactNode} from 'react';
import {Chevron} from '../shared/icon.tsx';
import {MarkdownText} from './markdown.tsx';

interface DisclosureProps {
  className?: string;
  open: boolean;
  onToggle: (open: boolean) => void;
  title?: string;
  summary: ReactNode;
  preview?: string;
  marker?: ReactNode;
  children?: ReactNode;
}

// Open content lines up under the heading text. A disclosure with a marker,
// such as a tool call's status, draws it in a column before the heading, and
// the column's width indents the content; one without draws a chevron after
// the heading.
export function Disclosure({className = '', open, onToggle, title, summary, preview, marker, children}: DisclosureProps) {
  return <Collapsible className={`disclosure [--disclosure-indent:--spacing(6)] ${className}`} open={open} onOpenChange={onToggle}>
    <CollapsibleTrigger asChild><Button variant="ghost" size="sm" className="h-auto w-full items-start justify-start gap-0 whitespace-normal px-0 py-1 text-left font-normal hover:bg-transparent hover:text-inherit dark:hover:bg-transparent" title={title}>
      {marker && <span className="disclosure-marker flex h-5 w-(--disclosure-indent) shrink-0 items-center">{marker}</span>}
      <span className="min-w-0 flex-1"><span className="disclosure-heading flex min-w-0 items-center gap-2">{summary}{!marker && <Chevron open={open} />}</span>
        {!open && preview && <span className="disclosure-preview block truncate leading-6 text-muted-foreground"><MarkdownText text={preview} /></span>}
      </span>
    </Button></CollapsibleTrigger>
    <CollapsibleContent className={`${slide} ${marker ? 'pl-(--disclosure-indent)' : ''}`}>{children}</CollapsibleContent>
  </Collapsible>;
}
