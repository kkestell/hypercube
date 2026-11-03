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
// acronym such as GPT-6.1. A provider prefix before a slash adds nothing to
// the harness beside it.
function displayName(value = '') {
  const words = value.split('/').pop().split(/[\s_-]+/).filter(Boolean);
  const names = words.map(titleCase);
  return names.map((name, i) => {
    if (i === 0) return name;
    return /^\d/.test(words[i]) && /^[A-Z]+$/.test(names[i - 1]) ? `-${name}` : ` ${name}`;
  }).join('');
}

export const profileLabel = profile =>
  [profile.harness, profile.model, profile.effort].filter(Boolean).map(displayName).join(' · ');
