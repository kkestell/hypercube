// Local image paths are relative to the Markdown file. A file outside its
// session's directory has an absolute path, and so do its images.
import type {EditorTarget} from '../editor/target.ts';
import {markdownFilePath} from './markdown-path.ts';

export function markdownImageURL(href: string, owner: EditorTarget, path: string): string | null {
  try {
    const image = new URL(href, 'file:///');
    if (['http:', 'https:'].includes(image.protocol)) return image.href;
    const target = markdownFilePath(href, path);
    return target === null ? null : '/image?' + new URLSearchParams({...owner, path: target}) + image.hash;
  } catch {
    return null;
  }
}
