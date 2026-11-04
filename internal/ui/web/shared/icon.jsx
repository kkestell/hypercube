// Lucide icons shared by the application actions.
import {ArrowDown, ArrowLeft, ArrowRight, ArrowUp, Check, ChevronDown, ChevronRight, CircleAlert,
  Clock, Copy, Eye, File, FileCode, FileMinus, FilePlus, Files, FolderPlus, GitBranch, GitCompare,
  Globe, Info, LoaderCircle, MessageSquare, MoreHorizontal, PanelBottom, PanelLeft, PanelRight,
  Paperclip, Plus, RefreshCw, Search, Send, Settings, Square, SquareArrowOutUpRight, TriangleAlert, WrapText, X} from 'lucide-react';

const icons = {
  'arrow-down': ArrowDown, 'arrow-left': ArrowLeft, 'arrow-right': ArrowRight, 'arrow-up': ArrowUp,
  wrap: WrapText, check: Check, 'chevron-down': ChevronDown, 'chevron-right': ChevronRight, close: X, copy: Copy,
  error: CircleAlert, warning: TriangleAlert, info: Info, history: Clock, loading: LoaderCircle,
  eye: Eye, files: Files, 'source-control': GitCompare, 'git-branch': GitBranch, globe: Globe,
  'comment-discussion': MessageSquare, 'new-folder': FolderPlus, add: Plus, ellipsis: MoreHorizontal,
  search: Search, settings: Settings, attach: Paperclip, send: Send, stop: Square, refresh: RefreshCw, 'link-external': SquareArrowOutUpRight,
  'layout-sidebar-left': PanelLeft, 'layout-sidebar-right': PanelRight, 'layout-panel': PanelBottom,
  'open-preview': Eye, 'go-to-file': FileCode, 'diff-modified': File, 'diff-added': FilePlus, 'diff-removed': FileMinus,
};

export function Icon({name, spin, className = '', title, ...props}) {
  const Component = icons[name];
  return <Component className={`size-4 shrink-0 ${spin ? 'animate-spin origin-center [transform-box:view-box]' : ''} ${className}`} data-icon={name} title={title} aria-label={title} aria-hidden={title || props['aria-label'] ? undefined : true} {...props} />;
}
