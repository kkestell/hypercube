// One line of text that, when too long for its room, fades out over its end
// instead of ending in an ellipsis, as sidebar rows do. In a row marked
// group/row that sets --cover to the width of controls shown over the text's
// end on hover or keyboard focus, the text then fades out before them.
import {useLayoutEffect, useRef, useState} from 'react';

// fade fades text out over --fade-width, 1.5rem unless set, ending --fade-end
// before its right edge.
export const fade = '[mask-image:linear-gradient(to_right,#000_calc(100%_-_var(--fade-end)_-_var(--fade-width,1.5rem)),transparent_calc(100%_-_var(--fade-end)))] [--fade-end:0px]';

const covered = 'group-hover/row:[--fade-end:var(--cover)] group-hover/row:[--fade-width:1rem] group-focus-visible/row:[--fade-end:var(--cover)] group-focus-visible/row:[--fade-width:1rem] group-has-[:focus-visible]/row:[--fade-end:var(--cover)] group-has-[:focus-visible]/row:[--fade-width:1rem]';

export function FadingText({text, className = ''}: {text: string; className?: string}) {
  const ref = useRef<HTMLSpanElement>(null);
  const [overflows, setOverflows] = useState(false);
  useLayoutEffect(() => {
    const element = ref.current;
    if (!element) return;
    const measure = () => setOverflows(element.scrollWidth > element.clientWidth);
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(element);
    return () => observer.disconnect();
  }, [text]);
  return <span ref={ref} className={`min-w-0 overflow-hidden whitespace-nowrap transition-[--fade-end,--fade-width] ${fade} ${overflows ? '[--fade-width:1rem]' : '[--fade-width:0px]'} ${covered} ${className}`}>{text}</span>;
}
