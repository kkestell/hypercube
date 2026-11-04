// Local sample interactions only; this mockup never calls the app's API.
const list = document.querySelector('.sidebar-list');
const dialog = document.querySelector('#session-dialog');

function select(button) {
  list.querySelectorAll('.session-item-selected').forEach(row => row.classList.remove('session-item-selected'));
  list.querySelectorAll('[aria-current]').forEach(item => item.removeAttribute('aria-current'));
  button.closest('.session-item').classList.add('session-item-selected');
  button.setAttribute('aria-current', 'true');
}

list.addEventListener('click', event => {
  const title = event.target.closest('.session-item-title');
  if (title) select(title);
});

document.querySelector('#new-session').addEventListener('click', () => dialog.showModal());
dialog.addEventListener('close', () => {
  if (dialog.returnValue !== 'add') return;
  const name = dialog.querySelector('input').value.trim();
  if (!name) return;
  const group = document.querySelector('#session-template').content.cloneNode(true);
  group.querySelector('.mockup-workspace-label').textContent = name;
  const button = group.querySelector('.session-item-title');
  list.append(group);
  select(button);
  button.focus();
  dialog.querySelector('form').reset();
});

// Changed files open their diff in an editor tab, or select it if open.
const tabs = document.querySelector('.editor-tabs');
const changes = document.querySelector('#scm-changes');

function selectTab(tab) {
  tabs.querySelectorAll('.editor-tabs-tab').forEach(other => {
    const active = other === tab;
    other.classList.toggle('editor-tabs-tab-active', active);
    other.querySelector('.editor-tabs-select').setAttribute('aria-selected', active);
    document.getElementById(other.dataset.panel).hidden = !active;
  });
  changes.querySelectorAll('[aria-current]').forEach(row => row.removeAttribute('aria-current'));
  changes.querySelector(`[data-panel="${tab.dataset.panel}"]`)?.setAttribute('aria-current', 'true');
}

function openDiff(row) {
  let tab = tabs.querySelector(`[data-panel="${row.dataset.panel}"]`);
  if (!tab) {
    tab = document.querySelector('#tab-template').content.firstElementChild.cloneNode(true);
    tab.dataset.panel = row.dataset.panel;
    tab.querySelector('.file-icon').replaceWith(row.querySelector('.file-icon').cloneNode(true));
    tab.querySelector('.editor-tabs-label').textContent = row.dataset.title;
    tab.querySelector('.editor-tabs-select').title = row.dataset.title;
    const close = tab.querySelector('.editor-tabs-close');
    close.title = close.ariaLabel = `Close ${row.dataset.title}`;
    tabs.append(tab);
  }
  selectTab(tab);
}

changes.addEventListener('click', event => {
  const row = event.target.closest('.mockup-scm-file');
  if (row) openDiff(row);
});

document.querySelector('.mockup-scm-group').addEventListener('click', ({currentTarget: group}) => {
  const open = group.ariaExpanded !== 'true';
  group.ariaExpanded = open;
  changes.hidden = !open;
});

tabs.addEventListener('click', event => {
  const tab = event.target.closest('.editor-tabs-tab');
  if (!tab) return;
  if (!event.target.closest('.editor-tabs-close')) return selectTab(tab);
  const wasActive = tab.classList.contains('editor-tabs-tab-active');
  document.getElementById(tab.dataset.panel).hidden = true;
  tab.remove();
  const next = tabs.querySelector('.editor-tabs-tab:last-child');
  if (wasActive && next) selectTab(next);
  if (!tabs.children.length) changes.querySelectorAll('[aria-current]').forEach(row => row.removeAttribute('aria-current'));
});
