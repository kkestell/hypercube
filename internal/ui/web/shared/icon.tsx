// Lucide icons shared by the application actions.
import {ArrowDown, BookMarked, ArrowLeft, ArrowLeftRight, ArrowRight, ArrowUp, Check, ChevronDown, ChevronRight, CircleAlert,
  Clock, CloudUpload, Copy, Folder, Eye, EyeOff, FileCode, Files, FolderPlus, GitBranch, GitCompare, GitMerge, GitPullRequest, GitPullRequestArrow, GitPullRequestDraft,
  Globe, Info, Laptop, MessageSquare, Paperclip, MoreHorizontal, PanelBottom, PanelLeft, PanelRight,
  Archive, Code, FoldHorizontal, FolderOpen, Pencil, Play, Plus, SquareTerminal, Trash2, RefreshCw, Search, Send, Settings, Square, SquareArrowOutUpRight, Undo2, TriangleAlert, User, WrapText, X, type LucideProps} from 'lucide-react';

const icons = {
  'arrow-down': ArrowDown, 'arrow-left': ArrowLeft, 'arrow-right': ArrowRight, 'arrow-up': ArrowUp,
  wrap: WrapText, 'fold-horizontal': FoldHorizontal, check: Check, 'chevron-down': ChevronDown, 'chevron-right': ChevronRight, close: X, copy: Copy,
  error: CircleAlert, warning: TriangleAlert, info: Info, history: Clock,
  'cloud-upload': CloudUpload, user: User, repo: BookMarked,
  eye: Eye, 'eye-off': EyeOff, files: Files, 'source-control': GitCompare, 'git-branch': GitBranch, 'git-merge': GitMerge, 'git-pull-request': GitPullRequest, 'git-pull-request-arrow': GitPullRequestArrow, 'git-pull-request-draft': GitPullRequestDraft, globe: Globe, laptop: Laptop,
  'comment-discussion': MessageSquare, 'new-folder': FolderPlus, add: Plus, attach: Paperclip, ellipsis: MoreHorizontal,
  search: Search, settings: Settings, send: Send, stop: Square, refresh: RefreshCw, 'link-external': SquareArrowOutUpRight,
  'layout-sidebar-left': PanelLeft, 'layout-sidebar-right': PanelRight, 'layout-panel': PanelBottom,
  'open-preview': Eye, 'go-to-file': FileCode,
  differs: ArrowLeftRight, play: Play, edit: Pencil, archive: Archive, discard: Undo2, trash: Trash2, code: Code, folder: Folder, 'folder-open': FolderOpen, terminal: SquareTerminal,
};

export type IconName = keyof typeof icons;

interface IconProps extends Omit<LucideProps, 'name'> {
  name: IconName;
  title?: string;
}

export function Icon({name, className = '', title, ...props}: IconProps) {
  const Component = icons[name];
  return <Component className={`size-4 shrink-0 ${className}`} data-icon={name} {...{title}} aria-label={title} aria-hidden={title || props['aria-label'] ? undefined : true} {...props} />;
}

// Chevron marks an expandable item, turning to point down as it opens.
export function Chevron({open}: {open: boolean}) {
  return <Icon name="chevron-right" className={`transition-transform ${open ? 'rotate-90' : ''}`} />;
}
