import {Button} from '../components/ui/button';
import {Collapsible, CollapsibleContent, CollapsibleTrigger} from '../components/ui/collapsible';
import type {ReactNode} from 'react';
import {Icon} from '../shared/icon.tsx';
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

export function Disclosure({className = '', open, onToggle, title, summary, preview, marker, children}: DisclosureProps) {
  return <Collapsible className={`disclosure ${className}`} open={open} onOpenChange={onToggle}>
    <CollapsibleTrigger asChild><Button variant="ghost" size="sm" className="h-auto w-full items-start justify-start whitespace-normal px-0 py-1 text-left font-normal hover:bg-transparent dark:hover:bg-transparent" title={title}>
      <span className="disclosure-marker flex h-5 shrink-0 items-center">{marker ?? <Icon name={open ? 'chevron-down' : 'chevron-right'} />}</span>
      <span className="min-w-0 flex-1"><span className="disclosure-heading flex min-w-0 items-center gap-2">{summary}</span>
        {!open && preview && <span className="disclosure-preview block truncate text-muted-foreground"><MarkdownText text={preview} /></span>}
      </span>
    </Button></CollapsibleTrigger>
    <CollapsibleContent>{children}</CollapsibleContent>
  </Collapsible>;
}
