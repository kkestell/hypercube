// normalizeAddress is the URL that text typed in an address bar names, or null
// when it names none. An address without a scheme uses https.
function normalizeAddress(address) {
  address = address.trim();
  if (!address.includes('://') && !address.startsWith('about:')) address = 'https://' + address;
  return URL.canParse(address) ? address : null;
}

module.exports = {normalizeAddress};
