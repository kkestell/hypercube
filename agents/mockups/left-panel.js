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
