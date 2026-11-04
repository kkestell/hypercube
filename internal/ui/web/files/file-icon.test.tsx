import {afterEach, expect, test} from 'bun:test';
import {render, unmountAll} from '../../test-render.tsx';
import {FileIcon} from './file-icon.tsx';

afterEach(unmountAll);
test('file icons are decorative Lucide icons for known and unknown files', () => {
  const root = document.createElement('div');
  for (const name of ['main.go', 'README.md', 'unknown.blob']) {
    render(<FileIcon name={name} />, root);
    expect(root.querySelector('svg')?.getAttribute('aria-hidden')).toBe('true');
  }
});
