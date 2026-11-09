// Profile snapshot for this prototype; edits stay in memory.
const profiles = [
  {
    "name": "opencode-deepseek-flash-max",
    "description": "DeepSeek V4.1 Flash at max effort for ordinary implementation, investigation, and review. Cheap and fast. Artificial Analysis: Terminal-Bench 4.0 26.8%; no FrontierCode 2.0 or cost per task listed.",
    "harness": "opencode",
    "model": "opencode-go/deepseek-v4.1-flash",
    "effort": "max"
  },
  {
    "name": "codex-gpt-luna-medium",
    "description": "GPT Luna 6 at medium effort only for trivial tasks such as lookups and quick checks. Weak at coding. Cheap and fast. Artificial Analysis: Terminal-Bench 4.0 2.5%, FrontierCode 2.0 12.8%, $0.04 per task.",
    "harness": "codex",
    "model": "gpt-6-luna",
    "effort": "medium"
  },
  {
    "name": "codex-gpt-terra-high",
    "description": "GPT Terra at high effort for implementation, design work, and live skill testing.",
    "harness": "codex",
    "model": "gpt-6.1-terra",
    "effort": "high"
  },
  {
    "name": "codex-gpt-sol-high",
    "description": "GPT Sol at high effort, only for code review and for implementation from a written plan. Its reviews include many speculative findings, so verify each one before acting on it. Never use it for design work: it over-engineers. Artificial Analysis: Terminal-Bench 4.0 51.5%, FrontierCode 2.0 36.7%, $0.59 per task.",
    "harness": "codex",
    "model": "gpt-6.1-sol",
    "effort": "high"
  },
  {
    "name": "codex-gpt-sol-xhigh",
    "description": "GPT Sol at extra high effort, only for hard code review and for hard implementation from a written plan. Its reviews include many speculative findings, so verify each one before acting on it. Never use it for design work: it over-engineers. Slower and costlier. Artificial Analysis: Terminal-Bench 4.0 54.0%, FrontierCode 2.0 38.5%, $0.76 per task.",
    "harness": "codex",
    "model": "gpt-6.1-sol",
    "effort": "xhigh"
  },
  {
    "name": "claude-haiku-xhigh",
    "description": "Haiku at extra high effort for small, well-defined tasks: lookups, simple edits, and quick checks. Artificial Analysis: Terminal-Bench 4.0 29.3%, FrontierCode 2.0 29.9%, $0.25 per task.",
    "harness": "claude",
    "model": "haiku",
    "effort": "xhigh"
  },
  {
    "name": "claude-sonnet-high",
    "description": "Sonnet at high effort for ordinary implementation, investigation, and review. Artificial Analysis: Terminal-Bench 4.0 43.9%, FrontierCode 2.0 25.7%, $1.64 per task.",
    "harness": "claude",
    "model": "sonnet",
    "effort": "high"
  },
  {
    "name": "claude-opus-medium",
    "description": "Opus at medium effort for hard implementation, debugging, and design work. Artificial Analysis: Terminal-Bench 4.0 52.5%, FrontierCode 2.0 33.1%, $2.45 per task.",
    "harness": "claude",
    "model": "opus",
    "effort": "medium"
  },
  {
    "name": "claude-opus-high",
    "description": "Opus at high effort for the hardest design and debugging work. Slower and costlier than claude-large. Artificial Analysis: Terminal-Bench 4.0 56.6%, FrontierCode 2.0 35.9%, $3.46 per task.",
    "harness": "claude",
    "model": "opus",
    "effort": "high"
  }
];

const dialog = document.querySelector('#settings');
const editor = document.querySelector('#profile-editor');
const profileList = document.querySelector('#profile-list');
const defaultProfile = document.querySelector('#default-profile');
const status = document.querySelector('#prototype-note');
let selectedProfile = profiles[2];

function renderProfiles() {
  const defaultName = defaultProfile.value;
  profileList.replaceChildren();
  defaultProfile.replaceChildren();
  for (const profile of profiles) {
    const button = document.createElement('button');
    button.type = 'button';
    button.textContent = profile.name || 'Untitled profile';
    button.setAttribute('aria-pressed', String(profile === selectedProfile));
    button.addEventListener('click', () => {
      selectedProfile = profile;
      renderProfiles();
    });
    profileList.append(button);
    defaultProfile.add(new Option(profile.name || 'Untitled profile', profile.name));
  }
  if (profiles.some(profile => profile.name === defaultName)) defaultProfile.value = defaultName;
  for (const field of ['name', 'harness', 'model', 'effort', 'description']) {
    editor.elements.namedItem(field).value = selectedProfile[field];
  }
  document.querySelector('[data-panel="profiles"] span').textContent = profiles.length;
  document.querySelector('#remove-profile').disabled = profiles.length === 1;
}

for (const button of document.querySelectorAll('[data-panel]')) {
  button.addEventListener('click', () => {
    for (const item of document.querySelectorAll('[data-panel]')) item.removeAttribute('aria-current');
    button.setAttribute('aria-current', 'page');
    for (const page of document.querySelectorAll('.page')) page.hidden = page.id !== button.dataset.panel;
    document.querySelector('.content').scrollTop = 0;
    document.querySelector(`#${button.dataset.panel} h3`).focus();
  });
}

editor.addEventListener('input', event => {
  const field = event.target.name;
  selectedProfile[field] = event.target.value;
  // Update the list label without rebuilding the focused form field.
  const index = profiles.indexOf(selectedProfile);
  profileList.children[index].textContent = selectedProfile.name || 'Untitled profile';
  defaultProfile.options[index].textContent = selectedProfile.name || 'Untitled profile';
  defaultProfile.options[index].value = selectedProfile.name;
});
editor.addEventListener('submit', event => event.preventDefault());

document.querySelector('#add-profile').addEventListener('click', () => {
  selectedProfile = {name: 'new-profile', harness: 'codex', model: '', effort: '', description: ''};
  profiles.push(selectedProfile);
  renderProfiles();
  editor.elements.namedItem('name').focus();
});
document.querySelector('#remove-profile').addEventListener('click', () => {
  profiles.splice(profiles.indexOf(selectedProfile), 1);
  selectedProfile = profiles[0];
  renderProfiles();
});

for (const button of document.querySelectorAll('[data-revoke]')) {
  button.addEventListener('click', () => {
    const row = button.closest('tr');
    const nextButton = row.nextElementSibling?.querySelector('button') || row.previousElementSibling?.querySelector('button');
    row.remove();
    const empty = document.querySelector('#grants').children.length === 0;
    document.querySelector('#empty-grants').hidden = !empty;
    document.querySelector('.table-wrap').hidden = empty;
    (nextButton || document.querySelector('#permissions-title')).focus();
    status.textContent = 'Sample permission removed. Real permissions are unchanged.';
  });
}

document.querySelector('#save').addEventListener('click', () => {
  status.textContent = 'Preview saved in this tab only. Reload to reset. Your config is unchanged.';
});
for (const button of document.querySelectorAll('[data-close]')) button.addEventListener('click', () => dialog.close());
const opener = document.querySelector('#open-settings');
opener.addEventListener('click', () => dialog.showModal());
dialog.addEventListener('close', () => opener.focus());
renderProfiles();
dialog.showModal();
