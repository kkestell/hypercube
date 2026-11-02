// Gives bun test a browser document, so components render into happy-dom.
import {GlobalRegistrator} from '@happy-dom/global-registrator';

GlobalRegistrator.register({url: 'http://localhost/'});
