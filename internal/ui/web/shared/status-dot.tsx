// A small dot, centered in an icon-sized box, whose color says what it means.
// Its title says the same in words. An outlined dot is hollow; an ended
// session's dot is always hollow.
import type {ReactNode} from 'react';
import {tones} from './tones.ts';

const colors = {waiting: tones.warning, unseen: tones.info, idle: tones.muted, ended: tones.muted};

// DotBox is the icon-sized box a dot sits in, which names it.
export function DotBox({title, className, children}: {title: string; className: string; children: ReactNode}) {
  return <span role="img" aria-label={title} title={title} className={`flex size-4 shrink-0 items-center justify-center ${className}`}>{children}</span>;
}

export const dotClass = (outline: boolean) => `size-2 rounded-full ${outline ? 'border border-current' : 'bg-current'}`;

export function StatusDot({kind, title, outline = false}: {kind: keyof typeof colors; title: string; outline?: boolean}) {
  return <DotBox title={title} className={colors[kind]}><span className={dotClass(outline || kind === 'ended')} /></DotBox>;
}
