// Local files have broader access in Electron than ordinary web pages.
function isBrowserURL(address) {
  if (!URL.canParse(address)) return false;
  const url = new URL(address);
  return url.protocol === 'http:' || url.protocol === 'https:' || url.href === 'about:blank';
}

// normalizeAddress is the supported URL that text typed in an address bar
// names, or null. An address without a scheme uses https.
function normalizeAddress(address) {
  address = address.trim();
  if (!address.includes('://') && !address.startsWith('about:')) address = 'https://' + address;
  return isBrowserURL(address) ? address : null;
}

module.exports = {isBrowserURL, normalizeAddress};
