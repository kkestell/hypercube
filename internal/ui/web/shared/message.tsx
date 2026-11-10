// A line of text shown in place of content: muted for empty and loading
// states, and an announced alert for errors. A filling message centers itself
// in a whole view, such as a file or diff that cannot be shown.
import type {ReactNode} from 'react';
import {cn} from 'cn';

interface MessageProps {
  error?: boolean;
  fill?: boolean;
  className?: string;
  children: ReactNode;
}

export function Message({error = false, fill = false, className, children}: MessageProps) {
  return <p role={error ? 'alert' : undefined} className={cn('text-sm break-words',
    error ? 'whitespace-pre-wrap text-destructive' : 'text-muted-foreground', fill && 'p-6 text-center', className)}>{children}</p>;
}
