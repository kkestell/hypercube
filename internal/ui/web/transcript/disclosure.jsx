// A transcript row that opens like a tree item: a one-line summary ending in a
// chevron, an optional preview line while closed, and its contents once open.
import './disclosure.css';

export function Disclosure({class: className, open, onToggle, title, summary, preview, children}) {
  return (
    <details class={className ? 'disclosure ' + className : 'disclosure'} open={open} onToggle={e => onToggle(e.currentTarget.open)}>
      <summary class="disclosure-summary" title={title}>
        <span class="disclosure-heading">
          {summary}
          <vscode-icon name="chevron-right" class="disclosure-chevron" />
        </span>
        {!open && preview && <span class="disclosure-preview">{preview}</span>}
      </summary>
      {children}
    </details>
  );
}
