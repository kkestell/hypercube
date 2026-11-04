import {useEffect, useRef, useState} from 'react';
import {Button} from '../components/ui/button';
import {Icon} from '../shared/icon.jsx';

export function CopyButton({text, label}) {
  const [copied, setCopied] = useState(false);
  const timer = useRef(null);
  useEffect(() => () => clearTimeout(timer.current), []);
  const copy = async () => {
    await navigator.clipboard.writeText(text);
    setCopied(true);
    clearTimeout(timer.current);
    timer.current = setTimeout(() => setCopied(false), 1500);
  };
  return <Button className="copy-button" variant="ghost" size="icon" type="button" aria-label={copied ? 'Copied' : label}
    title={copied ? 'Copied' : label} onClick={copy}><Icon name={copied ? 'check' : 'copy'} /></Button>;
}
