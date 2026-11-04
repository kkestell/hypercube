import {expect, test} from 'bun:test';
import {imageSrc} from './images.ts';

const png = 'iVBORw0KGgo=';

test('imageSrc accepts only base64 PNG, JPEG, GIF, and WebP', () => {
  expect(imageSrc({mime_type: 'image/png', data: png})).toBe('data:image/png;base64,' + png);
  expect(imageSrc({mime_type: 'image/webp', data: 'AAAA'})).toBe('data:image/webp;base64,AAAA');
  for (const bad of [
    {mime_type: 'image/svg+xml', data: png},
    {mime_type: 'text/html', data: png},
    {mime_type: 'image/png;x=1', data: png},
    {mime_type: 'image/png', data: 'a" onerror="x'},
    {mime_type: 'image/png', data: 'data:image/png;base64,' + png},
    {mime_type: 'image/png', data: ''},
  ]) expect(imageSrc(bad)).toBeNull();
});
