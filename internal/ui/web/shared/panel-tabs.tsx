// Panel content stays mounted so switching tabs preserves each view. The tab
// bar is on the window's top row; clearsWindowButtons makes room beside the
// window's buttons for a panel at the left edge.
import {Children, type ReactNode} from 'react';
import {Tabs, TabsContent, TabsList, TabsTrigger} from '../components/ui/tabs';
import {barTabs, titleBarMouseDown, topRow, topRowStart} from '../app/title-bar.ts';
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
  clearsWindowButtons?: boolean;
  children: ReactNode;
}

export function PanelTabs<Id extends string>({label, tabs, selected, onSelect, actions, clearsWindowButtons, children}: PanelTabsProps<Id>) {
  const views = Children.toArray(children);
  return <Tabs value={selected} onValueChange={value => { const tab = tabs.find(t => t.id === value); if (tab) onSelect?.(tab.id); }} className="panel-tabs flex h-full min-h-0 flex-col gap-0">
    <div className={`flex shrink-0 items-center justify-between gap-1 border-b border-black/12 bg-[color-mix(in_srgb,var(--background)_90%,black)] px-2 py-1 dark:border-white/7 ${topRow} ${clearsWindowButtons ? topRowStart : ''}`} onMouseDown={titleBarMouseDown}>
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
