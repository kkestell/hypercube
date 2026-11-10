// Panel content stays mounted so switching tabs preserves each view. The tab
// bar is on the window's top row; clearsWindowButtons makes room beside the
// window's buttons for a panel at the left edge.
import {Children, type ReactNode} from 'react';
import {Tabs, TabsContent, TabsList, TabsTrigger} from '../components/ui/tabs';
import {barTabs} from '../app/title-bar.ts';
import {Icon, type IconName} from './icon.tsx';
import {TopBar} from './top-bar.tsx';

// PanelTab is one tab: its ID, title, and icon, and a count shown beside the
// icon when it is above zero.
export interface PanelTab<Id extends string = string> {
  id: Id;
  title: string;
  icon: IconName;
  count?: number;
}

interface PanelTabsProps<Id extends string> {
  label: string;
  tabs: PanelTab<Id>[];
  selected: Id;
  onSelect?: (id: Id) => void;
  actions?: ReactNode;
  clearsWindowButtons?: boolean;
  children: ReactNode;
}

export function PanelTabs<Id extends string>({label, tabs, selected, onSelect, actions, clearsWindowButtons, children}: PanelTabsProps<Id>) {
  const views = Children.toArray(children);
  return <Tabs value={selected} onValueChange={value => { const tab = tabs.find(t => t.id === value); if (tab) onSelect?.(tab.id); }} className="flex h-full min-h-0 flex-col gap-0">
    <TopBar leftEdge={clearsWindowButtons ? 'always' : 'never'} actions={actions}>
      <TabsList aria-label={label} className={barTabs}>
        {tabs.map(tab => <TabsTrigger key={tab.id} value={tab.id} title={tab.title} aria-label={tab.count ? `${tab.title}, ${tab.count}` : tab.title}>
          <Icon name={tab.icon} />
          {!!tab.count && <span className="text-xs text-muted-foreground">{tab.count}</span>}
        </TabsTrigger>)}
      </TabsList>
    </TopBar>
    {tabs.map((tab, i) => <TabsContent key={tab.id} value={tab.id} forceMount hidden={selected !== tab.id}
      className="min-h-0 overflow-auto data-[state=inactive]:hidden">{views[i]}</TabsContent>)}
  </Tabs>;
}
