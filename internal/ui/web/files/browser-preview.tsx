// Browser viewers stream media directly instead of loading it into file state.
import {useState} from 'react';
import {Message} from '../shared/message.tsx';
import type {FilePreview} from './types.ts';

export function BrowserPreview({preview, path}: {preview: FilePreview; path: string}) {
  const [failed, setFailed] = useState(false);
  const {mime, url} = preview;
  const fallback = <Message fill>
    This browser could not display the file. <a className="underline" href={url} target="_blank" rel="noreferrer">Open file</a>
  </Message>;
  if (failed) return fallback;
  if (mime === 'text/html') return <iframe title={path} src={url} sandbox="" className="h-full w-full flex-1 border-0 bg-white" />;
  if (mime === 'application/pdf') return <object aria-label={path} data={url} type={mime} className="h-full w-full flex-1">{fallback}</object>;
  return <div className="flex min-h-0 flex-1 items-center justify-center overflow-auto p-4">
    {mime.startsWith('image/') ? <img src={url} alt={path} className="max-h-full max-w-full object-contain" onError={() => setFailed(true)} />
      : mime.startsWith('audio/') ? <audio aria-label={path} controls src={url} onError={() => setFailed(true)}>{fallback}</audio>
      : <video aria-label={path} controls src={url} className="max-h-full max-w-full" onError={() => setFailed(true)}>{fallback}</video>}
  </div>;
}
