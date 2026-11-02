// Open worker transcripts and files form a tree of tab panes and two-way splits.
export class TabLayout {
  constructor() {
    this.root = null;
  }

  pane(id) { return {tabs: [id], active: id}; }
  panes(node = this.root) {
    return !node ? [] : node.tabs ? [node] : node.children.flatMap(child => this.panes(child));
  }
  find(id) { return this.panes().find(p => p.tabs.includes(id)); }

  open(id) {
    let pane = this.find(id);
    if (!pane) {
      // Reuse the first pane, in left-to-right, top-to-bottom order.
      pane = this.panes()[0];
      if (pane) pane.tabs.push(id);
      else {
        pane = this.pane(id);
        this.root = pane;
      }
    }
    pane.active = id;
  }

  detach(id) {
    const pane = this.find(id);
    if (!pane) return;
    const index = pane.tabs.indexOf(id);
    pane.tabs.splice(index, 1);
    if (pane.active === id) pane.active = pane.tabs[Math.min(index, pane.tabs.length - 1)];
  }

  prune(node = this.root) {
    if (!node) return null;
    if (node.tabs) return node.tabs.length ? node : null;
    const children = node.children.map(child => this.prune(child)).filter(Boolean);
    if (children.length < 2) return children[0] ?? null;
    node.children = children;
    return node;
  }

  // replace puts a new tab in place of an open one, as a preview tab is replaced.
  replace(old, id) {
    const pane = this.find(old);
    pane.tabs[pane.tabs.indexOf(old)] = id;
    pane.active = id;
  }

  close(id) {
    this.detach(id);
    this.root = this.prune();
  }

  move(id, target, edge = 'center', before = null) {
    const source = this.find(id);
    if (!source || !this.panes().includes(target)) return;
    if (source === target && source.tabs.length === 1) return;
    if (before === id && source === target && edge === 'center') return;
    this.detach(id);
    if (edge === 'center') {
      const index = target.tabs.indexOf(before);
      target.tabs.splice(index < 0 ? target.tabs.length : index, 0, id);
      target.active = id;
    } else {
      const pane = this.pane(id);
      const branch = {axis: ['left', 'right'].includes(edge) ? 'x' : 'y', ratio: 0.5,
        children: ['left', 'top'].includes(edge) ? [pane, target] : [target, pane]};
      const replace = node => node === target ? branch : node.tabs ? node :
        Object.assign(node, {children: node.children.map(replace)});
      this.root = replace(this.root);
    }
    this.root = this.prune();
  }
}

// The pane DOM and transcript elements survive selection changes and moves.
export class TabContainer {
  constructor(root, layout, changed) {
    this.root = root;
    this.layout = layout;
    this.changed = changed;
    this.nodes = new Map();
    this.dragged = null;
  }

  render(entries) {
    this.entries = new Map(entries.map(entry => [entry.id, entry]));
    const used = new Set();
    const put = (parent, children) => {
      for (const child of [...parent.children]) if (!children.includes(child)) child.remove();
      children.forEach((child, i) => {
        if (parent.children[i] !== child) parent.insertBefore(child, parent.children[i] ?? null);
      });
    };
    const build = node => {
      used.add(node);
      let el = this.nodes.get(node);
      if (!el) {
        el = document.createElement('div');
        this.nodes.set(node, el);
        if (node.tabs) this.setupPane(el, node);
        else this.setupSplit(el, node);
      }
      if (!node.tabs) {
        el.style.flexDirection = node.axis === 'x' ? 'row' : 'column';
        const children = node.children.map(build);
        children[0].style.flex = `${node.ratio} 1 0%`;
        children[1].style.flex = `${1 - node.ratio} 1 0%`;
        put(el, [children[0], el.sash, children[1]]);
      } else {
        const tabs = node.tabs.map(id => {
          const entry = this.entries.get(id);
          let tab = [...el.bar.children].find(child => child.dataset.tab === id);
          if (!tab) {
            tab = document.createElement('div');
            tab.className = 'transcript-tab';
            tab.dataset.tab = id;
            tab.draggable = true;
            const select = document.createElement('button');
            select.type = 'button';
            select.setAttribute('role', 'tab');
            select.id = 'agent-tab-' + id;
            select.setAttribute('aria-controls', 'agent-panel-' + id);
            select.onclick = () => { node.active = id; this.changed(); select.focus(); };
            select.ondblclick = () => this.entries.get(id).keepOpen?.();
            if (entry.icon) select.insertAdjacentHTML('afterbegin', entry.icon);
            const label = document.createElement('span');
            label.className = 'tab-label';
            select.append(label);
            tab.label = label;
            tab.append(select);
            const close = document.createElement('button');
            close.type = 'button';
            close.className = 'close-tab';
            close.textContent = '×';
            close.title = `Close ${entry.label}`;
            close.setAttribute('aria-label', close.title);
            close.onclick = () => {
              this.layout.close(id);
              this.changed();
              const next = el.isConnected ? el.bar.querySelector('[aria-selected="true"]') :
                this.root.querySelector('[aria-selected="true"]');
              next?.focus();
            };
            tab.append(close);
          }
          const selected = id === node.active;
          tab.classList.toggle('active', selected);
          tab.classList.toggle('preview', !!entry.preview);
          const select = tab.firstElementChild;
          tab.label.textContent = entry.label;
          if (tab.status !== entry.status) {
            tab.status?.remove();
            tab.status = entry.status;
            if (entry.status) select.append(entry.status);
          }
          tab.firstElementChild.setAttribute('aria-selected', selected);
          tab.firstElementChild.title = entry.title ?? entry.label;
          tab.firstElementChild.tabIndex = selected ? 0 : -1;
          return tab;
        });
        put(el.bar, tabs);
        const bodies = node.tabs.map(id => {
          const element = this.entries.get(id).element;
          element.id = 'agent-panel-' + id;
          element.setAttribute('role', 'tabpanel');
          element.setAttribute('aria-labelledby', 'agent-tab-' + id);
          element.hidden = id !== node.active;
          return element;
        });
        put(el.body, bodies);
      }
      return el;
    };
    put(this.root, this.layout.root ? [build(this.layout.root)] : []);
    for (const [node, el] of this.nodes) {
      if (!used.has(node)) { el.remove(); this.nodes.delete(node); }
    }
  }

  setupSplit(el, node) {
    el.className = 'transcript-split';
    const sash = el.sash = document.createElement('div');
    sash.className = 'sash';
    sash.setAttribute('role', 'separator');
    sash.setAttribute('aria-label', 'Resize transcript panes');
    sash.setAttribute('aria-orientation', node.axis === 'x' ? 'vertical' : 'horizontal');
    sash.onpointerdown = e => {
      if (e.button !== 0) return;
      e.preventDefault();
      sash.setPointerCapture(e.pointerId);
      sash.classList.add('dragging');
      const move = e => {
        const rect = el.getBoundingClientRect();
        const ratio = node.axis === 'x' ? (e.clientX - rect.left) / rect.width : (e.clientY - rect.top) / rect.height;
        node.ratio = Math.max(0.15, Math.min(0.85, ratio));
        el.children[0].style.flex = `${node.ratio} 1 0%`;
        el.children[2].style.flex = `${1 - node.ratio} 1 0%`;
      };
      sash.onpointermove = move;
      sash.onlostpointercapture = () => { sash.onpointermove = null; sash.classList.remove('dragging'); };
      sash.onpointerup = e => sash.releasePointerCapture(e.pointerId);
    };
  }

  setupPane(el, pane) {
    el.className = 'transcript-pane';
    el.bar = document.createElement('div');
    el.bar.className = 'transcript-tabs';
    el.bar.setAttribute('role', 'tablist');
    el.bar.setAttribute('aria-label', 'Open tabs');
    el.body = document.createElement('div');
    el.body.className = 'transcript-body';
    el.append(el.bar, el.body);
    el.bar.onkeydown = e => {
      const tab = e.target.closest('[role="tab"]');
      if (!tab) return;
      const index = pane.tabs.indexOf(tab.parentElement.dataset.tab);
      let next;
      if (e.key === 'ArrowRight') next = (index + 1) % pane.tabs.length;
      else if (e.key === 'ArrowLeft') next = (index + pane.tabs.length - 1) % pane.tabs.length;
      else if (e.key === 'Home') next = 0;
      else if (e.key === 'End') next = pane.tabs.length - 1;
      else return;
      e.preventDefault();
      pane.active = pane.tabs[next];
      this.changed();
      el.bar.children[next].firstElementChild.focus();
    };
    el.bar.ondragstart = e => {
      const tab = e.target.closest('[data-tab]');
      if (!tab) return;
      this.dragged = tab.dataset.tab;
      e.dataTransfer.setData('text/plain', this.dragged);
      e.dataTransfer.effectAllowed = 'move';
    };
    const clear = () => {
      for (const paneEl of this.root.querySelectorAll('[data-drop]')) delete paneEl.dataset.drop;
    };
    el.bar.ondragend = () => { this.dragged = null; clear(); };
    const edgeAt = e => {
      if (el.bar.contains(e.target)) return 'center';
      const r = el.body.getBoundingClientRect();
      const x = (e.clientX - r.left) / r.width, y = (e.clientY - r.top) / r.height;
      const edges = [['left', x], ['right', 1 - x], ['top', y], ['bottom', 1 - y]].sort((a, b) => a[1] - b[1]);
      return edges[0][1] < 0.25 ? edges[0][0] : 'center';
    };
    el.ondragover = e => {
      if (!this.dragged) return;
      e.preventDefault();
      e.dataTransfer.dropEffect = 'move';
      clear();
      el.dataset.drop = edgeAt(e);
    };
    el.ondragleave = e => { if (!el.contains(e.relatedTarget)) delete el.dataset.drop; };
    el.ondrop = e => {
      if (!this.dragged) return;
      e.preventDefault();
      const before = el.bar.contains(e.target) ? e.target.closest('[data-tab]')?.dataset.tab : null;
      this.layout.move(this.dragged, pane, edgeAt(e), before);
      this.dragged = null;
      clear();
      this.changed();
    };
  }
}
