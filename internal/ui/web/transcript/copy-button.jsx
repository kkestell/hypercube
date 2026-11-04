import {useEffect, useRef, useState} from 'preact/hooks';
import './copy-button.css';

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
  return <button class="copy-button" type="button" aria-label={copied ? 'Copied' : label} title={copied ? 'Copied' : label} onClick={copy}><vscode-icon name={copied ? 'check' : 'copy'} /></button>;
}
