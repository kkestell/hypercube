// The tab bar above chat, a workspace dashboard, or the pull requests. Its
// one tab is always active and cannot be closed or moved.
import {LayoutToggles} from '../app/layout-toggles.tsx';
import {barTabs} from '../app/title-bar.ts';
import {Tabs, TabsList, TabsTrigger} from '../components/ui/tabs';
import {Icon, type IconName} from '../shared/icon.tsx';
import {TopBar} from '../shared/top-bar.tsx';

export function MainTabBar({icon, title}: {icon: IconName; title: string}) {
  return <TopBar leftEdge="withoutSidebar" className="border-b bg-tab" actions={<LayoutToggles />}>
    <Tabs className="min-w-0 flex-1" value="main">
      <TabsList className={`max-w-full justify-start ${barTabs}`}>
        <TabsTrigger className="min-w-0" value="main" title={title}>
          <Icon name={icon} />
          <span className="min-w-0 truncate">{title}</span>
        </TabsTrigger>
      </TabsList>
    </Tabs>
  </TopBar>;
}
