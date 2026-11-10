// A dot that breathes while work runs. Every dot's animation starts at the
// page timeline's origin, so all dots on the page breathe in step whenever
// they appear. A reduced-motion setting holds it still. An outlined dot is
// hollow.
import {useEffect, useRef} from 'react';
import {useReducedMotion} from './use-reduced-motion.ts';
import {DotBox, dotClass} from './status-dot.tsx';
import {tones} from './tones.ts';

const breath: Keyframe[] = [{opacity: 1}, {opacity: 0.4}];
const timing: KeyframeAnimationOptions = {duration: 1000, direction: 'alternate', iterations: Infinity, easing: 'ease-in-out'};

export function BusyDot({title, outline = false}: {title: string; outline?: boolean}) {
  const dot = useRef<HTMLSpanElement>(null);
  const reduced = useReducedMotion();
  useEffect(() => {
    if (reduced || !dot.current) return;
    const animation = dot.current.animate(breath, timing);
    animation.startTime = 0;
    // Canceling rejects the animation's finished promise.
    animation.finished.catch(() => {});
    return () => animation.cancel();
  }, [reduced]);
  return <DotBox title={title} className={tones.success}><span ref={dot} className={dotClass(outline)} /></DotBox>;
}
