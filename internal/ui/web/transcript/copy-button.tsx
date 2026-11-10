// CopyButton shows in the top right corner of its group on hover, and on its
// own keyboard focus, over a fade into the --copy-bg color its container sets,
// so text beneath it stays clear. Hidden, it stays in the tab order.
import {useEffect, useRef, useState} from 'react';
import {IconButton} from '../shared/icon-button.tsx';

interface CopyButtonProps {
  text: string;
  label: string;
}

export function CopyButton({text, label}: CopyButtonProps) {
  const [copied, setCopied] = useState(false);
  const timer = useRef<ReturnType<typeof setTimeout>>(undefined);
  useEffect(() => () => clearTimeout(timer.current), []);
  const copy = async () => {
    await navigator.clipboard.writeText(text);
    setCopied(true);
    clearTimeout(timer.current);
    timer.current = setTimeout(() => setCopied(false), 1500);
  };
  return <span className="absolute right-0 top-0 flex bg-linear-to-r from-transparent via-(--copy-bg) via-[length:1rem] to-(--copy-bg) pl-6 opacity-0 transition-opacity group-hover:opacity-100 has-[:focus-visible]:opacity-100">
    <IconButton icon={copied ? 'check' : 'copy'} label={copied ? 'Copied' : label} type="button" onClick={copy} />
  </span>;
}
