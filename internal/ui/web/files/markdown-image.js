// Local image paths are relative to the Markdown file. A file outside its
// session's directory has an absolute path, and so do its images.
export function markdownImageURL(href, session, path) {
  try {
    const base = new URL(path.split('/').map(encodeURIComponent).join('/'), 'file:///');
    const image = new URL(href, base);
    if (['http:', 'https:'].includes(image.protocol)) return image.href;
    // Only relative references use the local image route; explicit schemes
    // and network paths must not turn into local file reads.
    if (image.protocol !== 'file:' || /^[a-z][a-z\d+.-]*:/i.test(href.trim()) || href.startsWith('//')) return null;
    return '/image?' + new URLSearchParams({session, path: decodeURIComponent(path.startsWith('/') ? image.pathname : image.pathname.slice(1))}) + image.hash;
  } catch {
    return null;
  }
}
