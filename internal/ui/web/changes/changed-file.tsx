import type {ReactNode} from 'react';
import {FileIcon} from '../files/file-icon.tsx';
import {statusColor} from './changes.ts';
import type {ChangeStatus} from './types.ts';

export function ChangedFile({name, status, icon}: {name: string; status?: ChangeStatus; icon?: ReactNode}) {
  return <span className={`flex min-w-0 items-center gap-1.5 ${status ? statusColor[status] : ''}`}>
    {icon ?? <FileIcon name={name} />}
    <span className={`truncate ${status === 'deleted' ? 'line-through' : ''}`}>{name}</span>
  </span>;
}
