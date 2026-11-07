// Resolve local references against the Markdown file, not the app's URL.
// Paths in the session directory stay relative; outside files stay absolute.
export function markdownFilePath(href: string, path: string): string | null {
  try {
    const reference = href.trim().replace(/[\t\n\r]/g, '');
    const base = new URL(path.split('/').map(encodeURIComponent).join('/'), 'file:///');
    const target = new URL(reference, base);
    // Explicit schemes and network paths must not become local file reads.
    if (target.protocol !== 'file:' || target.host || /^[a-z][a-z\d+.-]*:/i.test(reference) || reference.startsWith('//')) return null;
    return decodeURIComponent(path.startsWith('/') ? target.pathname : target.pathname.slice(1));
  } catch {
    return null;
  }
}
