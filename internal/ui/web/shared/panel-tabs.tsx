// Panel content stays mounted so switching tabs preserves each view. The tab
// bar sits below the window's top row, which stays draggable like a title bar;
// outside the window there is no such row.
import {Children, type ReactNode} from 'react';
import {Tabs, TabsContent, TabsList, TabsTrigger} from '../components/ui/tabs';
import {barTabs, titleBarMouseDown, topRow} from '../app/title-bar.ts';
import {useStore} from '../store.ts';
import {Icon, type IconName} from './icon.tsx';

// PanelTab is one tab: its ID, title, and icon.
export interface PanelTab<Id extends string = string> {
  id: Id;
  title: string;
  icon: IconName;
}

interface PanelTabsProps<Id extends string> {
  label: string;
  tabs: PanelTab<Id>[];
  selected: Id;
  onSelect?: (id: Id) => void;
  actions?: ReactNode;
  children: ReactNode;
}

export function PanelTabs<Id extends string>({label, tabs, selected, onSelect, actions, children}: PanelTabsProps<Id>) {
  const [state] = useStore();
  const views = Children.toArray(children);
  return <Tabs value={selected} onValueChange={value => { const tab = tabs.find(t => t.id === value); if (tab) onSelect?.(tab.id); }} className="panel-tabs flex h-full min-h-0 flex-col gap-0">
    {state.titleBar && <div className={`shrink-0 border-b ${topRow}`} onMouseDown={titleBarMouseDown} />}
    <div className="flex shrink-0 items-center justify-between gap-1 border-b px-2 py-1">
      <TabsList aria-label={label} className={barTabs}>
        {tabs.map(tab => <TabsTrigger key={tab.id} value={tab.id} title={tab.title} aria-label={tab.title}>
          <Icon name={tab.icon} />
        </TabsTrigger>)}
      </TabsList>
      {actions && <div className="flex items-center">{actions}</div>}
    </div>
    {tabs.map((tab, i) => <TabsContent key={tab.id} value={tab.id} forceMount hidden={selected !== tab.id}
      className="min-h-0 overflow-auto data-[state=inactive]:hidden">{views[i]}</TabsContent>)}
  </Tabs>;
}
