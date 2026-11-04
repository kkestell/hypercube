// Panel content stays mounted so switching tabs preserves each view.
import {Children} from 'react';
import {Tabs, TabsContent, TabsList, TabsTrigger} from '../components/ui/tabs';
import {Icon} from './icon.jsx';

export function PanelTabs({label, tabs, selected, onSelect, actions, children}) {
  const views = Children.toArray(children);
  return <Tabs value={selected} onValueChange={onSelect} className="panel-tabs flex h-full min-h-0 flex-col gap-0">
    <div className="flex shrink-0 items-center justify-between gap-2 border-b p-2">
      <TabsList aria-label={label}>
        {tabs.map(tab => <TabsTrigger key={tab.id} value={tab.id} title={tab.title} aria-label={tab.title}>
          <Icon name={tab.icon} /><span>{tab.title}</span>
        </TabsTrigger>)}
      </TabsList>
      {actions && <div className="flex items-center gap-1">{actions}</div>}
    </div>
    {tabs.map((tab, i) => <TabsContent key={tab.id} value={tab.id} forceMount hidden={selected !== tab.id}
      className="min-h-0 overflow-auto data-[state=inactive]:hidden">{views[i]}</TabsContent>)}
  </Tabs>;
}
