import {useEffect, useRef, useState} from 'react';
import {Button} from '../components/ui/button';
import {Icon} from '../shared/icon.tsx';

interface CopyButtonProps {
  text: string;
  label: string;
  gutter?: boolean;
}

export function CopyButton({text, label, gutter}: CopyButtonProps) {
  const [copied, setCopied] = useState(false);
  const timer = useRef<ReturnType<typeof setTimeout>>(undefined);
  useEffect(() => () => clearTimeout(timer.current), []);
  const copy = async () => {
    await navigator.clipboard.writeText(text);
    setCopied(true);
    clearTimeout(timer.current);
    timer.current = setTimeout(() => setCopied(false), 1500);
  };
  return <Button className={`copy-button absolute top-0 ${gutter ? 'left-full' : 'right-0'} opacity-0 group-hover:opacity-100 group-focus-within:opacity-100`} variant="ghost" size="icon-xs" type="button" aria-label={copied ? 'Copied' : label}
    title={copied ? 'Copied' : label} onClick={copy}><Icon name={copied ? 'check' : 'copy'} /></Button>;
}
