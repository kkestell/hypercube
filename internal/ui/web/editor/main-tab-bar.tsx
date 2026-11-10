// The tab bar above chat, a workspace dashboard, or the pull requests. Its
// one tab is always active and cannot be closed or moved.
import {LayoutToggles} from '../app/layout-toggles.tsx';
import {barTabs, topRow, topRowStart} from '../app/title-bar.ts';
import {Tabs, TabsList, TabsTrigger} from '../components/ui/tabs';
import {Icon, type IconName} from '../shared/icon.tsx';
import {tabBackground} from '../shared/tab-background.ts';
import {useStore} from '../store.ts';

export function MainTabBar({icon, title}: {icon: IconName; title: string}) {
  const [state] = useStore();
  return <div className={`flex min-w-0 shrink-0 items-center justify-between gap-1 border-b px-2 ${tabBackground} ${topRow} ${!state.workbench.sidebar ? topRowStart : ''}`}>
    <Tabs className="min-w-0 flex-1" value="main">
      <TabsList className={`max-w-full justify-start ${barTabs}`}>
        <TabsTrigger className="min-w-0" value="main" title={title}>
          <Icon name={icon} />
          <span className="min-w-0 truncate">{title}</span>
        </TabsTrigger>
      </TabsList>
    </Tabs>
    <LayoutToggles />
  </div>;
}
