const keyNames = {ArrowDown: 'Down', ArrowUp: 'Up', ArrowLeft: 'Left', ArrowRight: 'Right', pagedown: 'PageDown', pageup: 'PageUp', '+': 'Plus'};

// accelerator is the Electron accelerator for a page's key binding,
// {key, metaKey, altKey, ctrlKey, shiftKey}, where key is lower case or a
// key name. A plus is already shifted.
function accelerator({key, metaKey, altKey, ctrlKey, shiftKey}) {
  const modifiers = [];
  if (metaKey) modifiers.push('Command');
  if (ctrlKey) modifiers.push('Control');
  if (altKey) modifiers.push('Alt');
  if (shiftKey && key !== '+') modifiers.push('Shift');
  return [...modifiers, keyNames[key] ?? key[0].toUpperCase() + key.slice(1)].join('+');
}

module.exports = {accelerator};
