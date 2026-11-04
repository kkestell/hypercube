// Lucide icons shared by the application actions.
import {ArrowDown, ArrowLeft, ArrowRight, ArrowUp, Check, ChevronDown, ChevronRight, CircleAlert,
  Clock, Copy, Eye, FileCode, Files, FolderPlus, GitBranch, GitCompare, GitMerge, GitPullRequestArrow,
  Globe, Info, LoaderCircle, MessageSquare, MoreHorizontal, PanelBottom, PanelLeft, PanelRight,
  Archive, Code, FoldHorizontal, FolderOpen, Paperclip, Pencil, Play, Plus, SquareTerminal, Trash2, RefreshCw, Search, Send, Settings, Square, SquareArrowOutUpRight, TriangleAlert, WrapText, X, type LucideProps} from 'lucide-react';

const icons = {
  'arrow-down': ArrowDown, 'arrow-left': ArrowLeft, 'arrow-right': ArrowRight, 'arrow-up': ArrowUp,
  wrap: WrapText, 'fold-horizontal': FoldHorizontal, check: Check, 'chevron-down': ChevronDown, 'chevron-right': ChevronRight, close: X, copy: Copy,
  error: CircleAlert, warning: TriangleAlert, info: Info, history: Clock, loading: LoaderCircle,
  eye: Eye, files: Files, 'source-control': GitCompare, 'git-branch': GitBranch, 'git-merge': GitMerge, 'git-pull-request-arrow': GitPullRequestArrow, globe: Globe,
  'comment-discussion': MessageSquare, 'new-folder': FolderPlus, add: Plus, ellipsis: MoreHorizontal,
  search: Search, settings: Settings, attach: Paperclip, send: Send, stop: Square, refresh: RefreshCw, 'link-external': SquareArrowOutUpRight,
  'layout-sidebar-left': PanelLeft, 'layout-sidebar-right': PanelRight, 'layout-panel': PanelBottom,
  'open-preview': Eye, 'go-to-file': FileCode,
  play: Play, edit: Pencil, archive: Archive, trash: Trash2, code: Code, 'folder-open': FolderOpen, terminal: SquareTerminal,
};

export type IconName = keyof typeof icons;

interface IconProps extends Omit<LucideProps, 'name'> {
  name: IconName;
  spin?: boolean;
  title?: string;
}

export function Icon({name, spin, className = '', title, ...props}: IconProps) {
  const Component = icons[name];
  return <Component className={`size-4 shrink-0 ${spin ? '[&>*]:origin-center [&>*]:animate-spin [&>*]:[transform-box:view-box]' : ''} ${className}`} data-icon={name} {...{title}} aria-label={title} aria-hidden={title || props['aria-label'] ? undefined : true} {...props} />;
}
