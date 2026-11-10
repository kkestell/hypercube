// Gives bun test a browser document, so components render into happy-dom.
import {GlobalRegistrator} from '@happy-dom/global-registrator';
import {expect} from 'bun:test';

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
// properties reach React's whole component tree, which takes minutes to print.
Object.defineProperty(Node.prototype, Bun.inspect.custom, {value(this: Node) {
  if (!(this instanceof Element)) return this.nodeName;
  return `<${this.localName}${[...this.attributes].map(({name, value}) => ` ${name}="${value}"`).join('')}>`;
}});

// Bun's equality diffs walk every property and skip the form above, so
// comparisons that hold page nodes print each side with Bun.inspect instead.
const holdsNode = (value: unknown): boolean => value instanceof Node
  || (Array.isArray(value) && value.some(holdsNode))
  || (value?.constructor === Object && Object.values(value).some(holdsNode));
const matchers = Object.getPrototypeOf(expect(null));
for (const [name, strict] of [['toEqual', false], ['toStrictEqual', true]] as const) {
  const builtIn = matchers[name];
  expect.extend({[name](received: unknown, expected: unknown) {
    if (!holdsNode(received) && !holdsNode(expected)) {
      try {
        builtIn.call(expect(received), expected);
        return {pass: true, message: () => `expected ${Bun.inspect(received)} not to equal ${Bun.inspect(expected)}`};
      } catch (error) {
        return {pass: false, message: () => (error as Error).message};
      }
    }
    const pass = Bun.deepEquals(received, expected, strict);
    return {pass, message: () => `expected ${Bun.inspect(received)} ${pass ? 'not ' : ''}to equal ${Bun.inspect(expected)}`};
  }});
}
