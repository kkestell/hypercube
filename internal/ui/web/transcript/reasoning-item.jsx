// Thinking: a header that counts how long it lasted. Expanded, a box five lines
// tall shows the newest text and scrolls back through the rest.
import {useLayoutEffect, useRef} from 'preact/hooks';
import {Disclosure} from './disclosure.jsx';
import {Elapsed} from './elapsed.jsx';
import './reasoning-item.css';

// The box follows the end of the text until the user scrolls up, and follows
// again when they scroll back to the end.
function ReasoningText({text}) {
  const box = useRef(null), following = useRef(true);
  useLayoutEffect(() => {
    if (following.current) box.current.scrollTop = box.current.scrollHeight;
  }, [text]);
  const scrolled = () => {
    const {scrollTop, clientHeight, scrollHeight} = box.current;
    following.current = scrollHeight - scrollTop - clientHeight < 2;
  };
  return <div ref={box} class="reasoning-item-text" onScroll={scrolled}>{text}</div>;
}

export function ReasoningItem({row, expanded, setOpen}) {
  const {item, closed, until} = row;
  const summary = <Elapsed live={!closed} present="Thinking" past="Thought" start={item.at} until={until} />;
  return (
    <div class="reasoning-item">
      <Disclosure open={expanded} onToggle={open => setOpen(item.id, open)} summary={summary}>
        {expanded && <ReasoningText text={item.text ?? ''} />}
      </Disclosure>
    </div>
  );
}
