import {Button} from '../components/ui/button';
import {Collapsible, CollapsibleContent, CollapsibleTrigger} from '../components/ui/collapsible';
import {Icon} from '../shared/icon.jsx';

export function Disclosure({className = '', open, onToggle, title, summary, preview, children}) {
  return <Collapsible className={`disclosure ${className}`} open={open} onOpenChange={onToggle}>
    <CollapsibleTrigger asChild><Button variant="ghost" className="h-auto w-full justify-start whitespace-normal text-left" title={title}>
      <Icon name={open ? 'chevron-down' : 'chevron-right'} />
      <span className="min-w-0"><span className="disclosure-heading flex items-center gap-2">{summary}</span>
        {!open && preview && <span className="disclosure-preview block truncate text-muted-foreground">{preview}</span>}
      </span>
    </Button></CollapsibleTrigger>
    <CollapsibleContent>{children}</CollapsibleContent>
  </Collapsible>;
}
