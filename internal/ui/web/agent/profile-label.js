// A profile's harness, model, and effort in their display form. The config
// holds lowercase IDs; known words get their proper spelling and everything
// else falls back to title case.

const properNames = {
  claude: 'Claude',
  codex: 'Codex',
  deepseek: 'DeepSeek',
  gpt: 'GPT',
  opencode: 'OpenCode',
  pi: 'Pi',
  xhigh: 'Extra High',
};

const titleCase = word => properNames[word.toLowerCase()] ?? word.charAt(0).toUpperCase() + word.slice(1);

// displayName turns a raw ID into words, keeping the hyphen in a versioned
// acronym such as GPT-6.1.
function displayName(value = '') {
  const words = value.split(/[\s/_-]+/).filter(Boolean);
  const names = words.map(titleCase);
  return names.map((name, i) => {
    if (i === 0) return name;
    return /^\d/.test(words[i]) && /^[A-Z]+$/.test(names[i - 1]) ? `-${name}` : ` ${name}`;
  }).join('');
}

// The first segment of a model ID names its provider. Keep the rest of the
// path, which may contain slashes of its own.
function modelName(model = '') {
  const provider = model.indexOf('/');
  return displayName(provider < 0 ? model : model.slice(provider + 1));
}

export const profileLabel = profile =>
  [displayName(profile.harness), modelName(profile.model), displayName(profile.effort)]
    .filter(Boolean)
    .join(' · ');
