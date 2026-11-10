import {useEffect, useRef, useState} from 'react';
import {Button} from '../components/ui/button';
import {Icon} from '../shared/icon.tsx';

// CopyButton shows in the top right corner of its group on hover, over a fade
// into the --copy-bg color its container sets, so text beneath it stays clear.

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
  return <span className="copy-button absolute right-0 top-0 flex bg-linear-to-r from-transparent via-(--copy-bg) via-[length:1rem] to-(--copy-bg) pl-6 opacity-0 transition-opacity group-hover:opacity-100 group-focus-within:opacity-100">
    <Button variant="ghost" size="icon-xs" type="button" aria-label={copied ? 'Copied' : label}
      title={copied ? 'Copied' : label} onClick={copy}><Icon name={copied ? 'check' : 'copy'} /></Button>
  </span>;
}
