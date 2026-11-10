// The titled sections of an item's details, such as a tool call's arguments
// and output, in one card.
import type {ReactNode} from 'react';
import {Card, CardContent} from '../components/ui/card';

export function TranscriptDetails({sections}: {sections: [title: string, content: ReactNode][]}) {
  return <Card className="gap-0 overflow-hidden py-0">
    {sections.map(([title, content], i) => <CardContent key={title} className={`space-y-2 px-3 py-2 ${i ? 'border-t' : ''}`}>
      <h4 className="text-xs font-medium text-muted-foreground">{title}</h4>
      {content}
    </CardContent>)}
  </Card>;
}
