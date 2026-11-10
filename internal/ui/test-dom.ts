// Gives bun test a browser document, so components render into happy-dom.
import {GlobalRegistrator} from '@happy-dom/global-registrator';

GlobalRegistrator.register({url: 'http://localhost/'});

// happy-dom lacks the CSS Custom Highlight API; a highlight is a set of ranges.
class TestHighlight extends Set<AbstractRange> {
  priority = 0;
  type = 'highlight';
  constructor(...ranges: AbstractRange[]) {
    super(ranges);
  }
}
Object.defineProperty(globalThis, 'Highlight', {value: TestHighlight});
Object.defineProperty(globalThis, 'CSS', {value: Object.assign(globalThis.CSS, {highlights: new Map<string, Highlight>()})});

declare global {
  var IS_REACT_ACT_ENVIRONMENT: boolean;
}
globalThis.IS_REACT_ACT_ENVIRONMENT = true;

// Failure messages show a page node as its opening tag. A node's own
// properties reach React's whole component tree, which takes seconds to print.
Object.defineProperty(Node.prototype, Bun.inspect.custom, {value(this: Node) {
  if (!(this instanceof Element)) return this.nodeName;
  return `<${this.localName}${[...this.attributes].map(({name, value}) => ` ${name}="${value}"`).join('')}>`;
}});
