// Gives bun test a browser document, so components render into happy-dom.
import {GlobalRegistrator} from '@happy-dom/global-registrator';

GlobalRegistrator.register({url: 'http://localhost/'});

// happy-dom lacks the CSS Custom Highlight API.
globalThis.Highlight = class { constructor(...ranges) { this.ranges = ranges; } };
Object.defineProperty(globalThis, 'CSS', {value: Object.assign(globalThis.CSS, {highlights: new Map()})});

globalThis.IS_REACT_ACT_ENVIRONMENT = true;
