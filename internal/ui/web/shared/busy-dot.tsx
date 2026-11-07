// A dot that breathes from gray to full color while work runs. Every dot's
// animation starts at the page timeline's origin, so all dots on the page
// breathe in step whenever they appear. An outlined dot is hollow.
const breath: Keyframe[] = [{opacity: 0.3}, {opacity: 1}];
const timing: KeyframeAnimationOptions = {duration: 1000, direction: 'alternate', iterations: Infinity, easing: 'ease-in-out'};

function breathe(dot: HTMLElement | null) {
  if (dot) dot.animate(breath, timing).startTime = 0;
}

export function BusyDot({title, outline = false}: {title: string; outline?: boolean}) {
  return <span role="img" aria-label={title} title={title} className="busy-dot flex size-4 shrink-0 items-center justify-center">
    <span ref={breathe} className={`size-2 rounded-full ${outline ? 'border border-foreground' : 'bg-foreground'}`} />
  </span>;
}
