#!/usr/bin/env python3
"""Print a mockup of Hypercube's screen: the layout, styles, and icons, with
the scheduled tasks of agents/plans/2026-10-28-003-scheduled-tasks.md.

Mirrors host.rs (layout, TeamChrome, draw_label, draw_row), bar.rs,
overlay.rs, and the planned tasklist.rs. Pane contents are blank. A thin gray border drawn
outside the canvas shows where it ends. `--icons` picks a set from ICONS.

The workers cycle through the states in WORKERS, so a few of them show every
note and icon. The workspace has the tasks in TASKS, each run by its own team;
`--view` shows a task's team, and `--teams` draws the list of teams Ctrl-b t
opens.

Colors mean the same everywhere: amber waits for the user, red is an error,
green is running, and gray is idle or stopped.

    ./mockup.py --cols 100 --rows 30 --workers 4 --focus 1 --icons nerd
    ./mockup.py --view 2 --icons nerd
    ./mockup.py --teams --icons nerd
    ./mockup.py --overlay --icons nerd
    ./mockup.py --sidebar --icons nerd
    ./mockup.py --sidebar --sidebar-focus --selected 2 --icons nerd

`--sidebar` replaces the header, the bar, the workspace overlay, and the team
list with a sidebar: every workspace, the current one's teams, and the viewed
team's agents.
"""
import argparse

RESET = "\x1b[0m"
BOLD = "\x1b[1m"
FRAME = "\x1b[0;38;2;100;100;100m"
HARNESS = "\x1b[0;48;2;40;46;64m"
# Waiting for a tool approval.
ATTENTION = (240, 170, 40)
# Running, including a busy agent's dot in its title.
RUNNING = (110, 190, 90)
# Idle or stopped, including an idle agent's dot in its title.
QUIET = (140, 140, 140)
# A harness that failed to start, a program that exited, a failed task.
ERROR = (230, 80, 70)
# The state of a workspace or team in the overlay and the team list: its
# icon's color.
STATES = {
    "running": RUNNING,
    "idle": QUIET,
    "stopped": QUIET,
    "failed": ERROR,
}
# The main team's name in the team list and the header.
MAIN = "main"
# Mirrors overlay.rs.
HELP = "\x1b[0;2m"
# Mirrors colors.rs. The mockups use the gray accent, the last.
ACCENTS = [
    (70, 160, 220),
    (40, 180, 170),
    (100, 180, 80),
    (220, 170, 50),
    (230, 120, 50),
    (220, 80, 80),
    (220, 90, 170),
    (150, 110, 220),
    (160, 160, 160),
]

# Each icon set. "nerd" is what Hypercube draws, with a Nerd Font; "plain" needs none.
# Notes take a format string with the number, if any, as {}.
ICONS = {
    "plain": {
        "expanded": "▾",
        "collapsed": "▸",
        "branch": "⎇",
        "directory": "",
        "activity": "●",
        "attention": "●",
        "zoomed": "zoomed",
        "exited": "exited {}",
        "failed": "failed:",
        "scrolled": "↑{}",
        "header": "",
        "running": "▶",
        "stopped": "■",
        "task": "›",
        "tasks": "",
        "idle": "‖",
        "task_failed": "✕",
    },
    "nerd": {
        "expanded": "",  # nf-oct-chevron_down
        "collapsed": "",  # nf-oct-chevron_right
        "branch": "",  # nf-dev-git_branch
        "directory": "",  # nf-fa-folder
        "activity": "",  # nf-fa-circle
        "attention": "",  # nf-fa-bell
        "zoomed": "",  # nf-fa-expand
        "exited": " {}",  # nf-fa-times_circle
        "failed": "",  # nf-fa-warning
        "scrolled": "↑{}",
        "header": "",  # nf-fa-folder_open
        "running": "",  # nf-fa-play
        "stopped": "",  # nf-fa-stop
        "task": "›",
        "tasks": "",  # nf-fa-clock_o
        "idle": "",  # nf-fa-pause
        "task_failed": "",  # nf-fa-warning
    },
}

# What each worker shows, cycling: whether it is busy, waits for an approval,
# rows scrolled, exit status, whether it is collapsed, a failure, and its
# branch, or its directory when the branch is empty.
WORKERS = [
    dict(busy=True, attention=True, scrolled=42, branch="feature-{}"),
    dict(busy=True, status=3, branch="feature-{}"),
    dict(busy=False, collapsed=True, branch="feature-{}"),
    dict(failed="opencode: not found", directory="~/src/docs"),
    dict(branch="feature-{}"),
]

# The workspace's tasks, in creation order: each one's name, cron schedule,
# state, workers, last run, and whether one of its agents waits for an
# approval. Its orchestrator is named after it.
TASKS = [
    dict(name="nightly-deps", schedule="0 3 * * *", state="idle", workers=0, last="9h ago"),
    dict(name="triage", schedule="*/30 9-17 * * 1-5", state="running", workers=2, last="4m ago", attention=True),
    dict(name="flaky-tests", schedule="0 */6 * * *", state="failed", workers=0, last="never"),
]


def mix(c, to, amount):
    return tuple(int(a + (b - a) * amount + 0.5) for a, b in zip(c, to))


def fg(c):
    return "\x1b[0;38;2;{};{};{}m".format(*c)


def sgr(fg, bg):
    return "\x1b[0;38;2;{};{};{};48;2;{};{};{}m".format(*fg, *bg)


def icon(icons, name, text):
    """`text` after the named icon, or alone if the set has none."""
    i = icons[name]
    return f"{i} {text}" if i else text


class Palette:
    """The chrome's colors, all shades of the accent numbered `n`."""

    def __init__(self, n):
        accent = ACCENTS[n % len(ACCENTS)]
        white, black = (255, 255, 255), (0, 0, 0)
        self.line = accent
        self.text = mix(accent, white, 0.4)
        self.bar_fg, self.bar_bg = mix(accent, white, 0.85), mix(accent, black, 0.78)
        # The selected row of the overlay and the task list.
        self.selected_fg, self.selected_bg = self.bar_fg, mix(accent, black, 0.55)


class Canvas:
    """A grid of cells; each is (char, sgr)."""

    def __init__(self, cols, rows):
        self.cols, self.rows = cols, rows
        self.cells = [[(" ", RESET)] * cols for _ in range(rows)]

    def put(self, row, col, text, sgr):
        for i, ch in enumerate(text):
            if 0 <= row < self.rows and 0 <= col + i < self.cols:
                self.cells[row][col + i] = (ch, sgr)

    def render(self, border):
        lines = []
        if border:
            lines.append(f"{FRAME}┌{'─' * self.cols}┐{RESET}")
        for row in self.cells:
            out, prev = [], None
            for ch, sgr in row:
                if sgr != prev:
                    out.append(sgr)
                    prev = sgr
                out.append(ch)
            body = "".join(out) + RESET
            lines.append(f"{FRAME}│{RESET}{body}{FRAME}│{RESET}" if border else body)
        if border:
            lines.append(f"{FRAME}└{'─' * self.cols}┘{RESET}")
        return "\n".join(lines)


def shares(total, weights):
    """`total` split by `weights`, rounded so the parts add up to it."""
    if not any(weights):
        weights = [1] * len(weights)
    out, before = [], 0
    for w in weights:
        start = total * before // sum(weights)
        before += w
        out.append(total * before // sum(weights) - start)
    return out


def layout(x, y, cols, rows, workers):
    """Each pane's (col, row, cols, rows), the label row above `row`, in the
    area of `cols` by `rows` at column `x` and row `y`. `workers` holds
    whether each is collapsed. Mirrors host::layout with even weights."""
    left = (cols - 1) // 2 if workers else cols
    rects = [(x, y + 1, left, rows - 1)]
    expanded = [w for w in workers if not w]
    extras = iter(shares(rows - len(workers) - len(expanded), [1] * len(expanded)))
    top = y
    for collapsed in workers:
        height = 0 if collapsed else 1 + next(extras)
        rects.append((x + left + 1, top + 1, cols - left - 1, height))
        top += 1 + height
    return rects


def draw_label(c, pal, icons, rect, pane, focused, zoomed, worker):
    """A title drawn on a horizontal line: ━ ▾ name ● 🔔 · notes ━━━ detail ━.
    Mirrors host::draw_label and draw_row."""
    col, row, width, height = rect
    row -= 1
    c.put(row, col, "━" * width, fg(pal.line))
    bold = BOLD if focused else ""
    style = fg(pal.text) + bold
    left = f" {icons['collapsed' if height == 0 else 'expanded']} {pane['name']}" if worker else f" {pane['name']}"
    # Spans of `left` drawn in another color: (start, text, color).
    spans = []
    if pane.get("failed"):
        left += " · "
        spans.append((len(left), f"{icons['failed']} {pane['failed']}", ERROR))
        left += spans[-1][1]
    room = max(width - 2, 0)
    if (
        pane.get("status") is None
        and not pane.get("failed")
        and pane.get("busy") is not None
        and len(left) + 2 <= room
    ):
        left += " "
        spans.append((len(left), icons["activity"], RUNNING if pane["busy"] else QUIET))
        left += icons["activity"]
    if pane.get("attention") and len(left) + 2 <= room:
        left += " "
        spans.append((len(left), icons["attention"], ATTENTION))
        left += icons["attention"]
    notes = []
    if zoomed:
        notes.append((icons["zoomed"], None))
    if pane.get("status") is not None:
        notes.append((icons["exited"].format(pane["status"]), ERROR))
    if pane.get("scrolled"):
        notes.append((icons["scrolled"].format(pane["scrolled"]), None))
    for note, color in notes:
        left += " · "
        if color:
            spans.append((len(left), note, color))
        left += note
    left = (left + " ")[:room]
    c.put(row, col + 1, left, style)
    # As much of each span as fits.
    for at, text, color in spans:
        c.put(row, col + 1 + at, left[at : at + len(text)], fg(color) + bold)
    right = f" {pane['detail']} " if pane["detail"] else ""
    if right and len(left) + 1 + len(right) <= room:
        c.put(row, col + width - 1 - len(right), right, style)


def team(icons, args, task):
    """The panes of the main team, or of `task`'s team: its orchestrator,
    named after the task, and its workers."""
    lead = dict(name="orchestrator", detail=icon(icons, "branch", "main"), busy=False)
    workers, prefix = args.workers, "worker-"
    if task:
        lead["name"] = task["name"]
        lead["busy"] = task["state"] == "running"
        if task["state"] == "failed":
            lead["failed"] = "claude: not found"
        workers, prefix = task["workers"], f"{task['name']}-"
    panes = [lead]
    for i in range(1, workers + 1):
        w = dict(WORKERS[(i - 1) % len(WORKERS)], name=f"{prefix}{i}")
        if w.get("branch"):
            w["detail"] = icon(icons, "branch", w["branch"].format(i))
        else:
            w["detail"] = icon(icons, "directory", w["directory"])
        panes.append(w)
    return panes


def draw_panes(c, pal, icons, args, panes, rects, right, top, rows):
    """The panes, their titles, and the divider, which follows the
    orchestrator from row `top` for `rows` rows unless it reaches column
    `right`."""
    divider = fg(pal.line)
    first = rects[0]
    if first[2] and first[0] + first[2] < right:
        for row in range(top, top + rows):
            c.put(row, first[0] + first[2], "┃", divider)
    for i, (pane, rect) in enumerate(zip(panes, rects)):
        col, row, width, height = rect
        if not width:
            continue
        for r in range(row, row + height):
            c.put(r, col, " " * width, HARNESS)
        draw_label(c, pal, icons, rect, pane, i == args.focus, i == args.zoom, i > 0 and args.zoom is None)
        if i > 0 and args.zoom is None:
            # The top titles meet at ┳; lower worker titles branch at ┣.
            c.put(row - 1, col - 1, "┳" if row == top + 1 else "┣", divider)


def draw(args):
    cols, rows = args.cols, args.rows
    icons = ICONS[args.icons]
    c = Canvas(cols, rows)
    pal = Palette(args.accent)
    bar = sgr(pal.bar_fg, pal.bar_bg)
    if args.overlay:
        return draw_overlay(c, pal, icons, args)
    if args.teams:
        return draw_teams(c, pal, icons, args)

    task = TASKS[args.view - 1] if args.view else None
    panes = team(icons, args, task)
    # The panes' area: between the header and the bar, or right of the
    # sidebar and the blank column that pads it.
    if args.sidebar:
        x, y, area_cols, area_rows = args.sidebar + 1, 0, cols - args.sidebar - 1, rows
    else:
        x, y, area_cols, area_rows = 0, 1, cols, rows - 2
    if args.zoom is not None:
        rects = [(0, 0, 0, 0)] * len(panes)
        rects[args.zoom] = (x, y + 1, area_cols, area_rows - 1)
    else:
        rects = layout(x, y, area_cols, area_rows, [bool(p.get("collapsed")) for p in panes[1:]])
    draw_panes(c, pal, icons, args, panes, rects, x + area_cols, y, area_rows)
    if args.sidebar:
        draw_sidebar(c, pal, icons, args, panes, task)
        return c.render(not args.no_border)

    # The header, in the bar colors: the workspace path and the viewed task's
    # name, and the task counts at the right while the workspace has tasks,
    # followed by the other teams waiting for an approval, each with an
    # attention icon.
    c.put(0, 0, " " * cols, bar)
    left = f" {icon(icons, 'header', args.path)}"
    if task:
        left += f" {icons['task']} {task['name']}"
    c.put(0, 0, left[:cols], bar)
    if TASKS and not args.no_tasks:
        running = sum(t["state"] == "running" for t in TASKS)
        noun = "task" if len(TASKS) == 1 else "tasks"
        right = icon(icons, "tasks", f"{len(TASKS)} {noun}, {running} running")
        waiting = [t["name"] for t in TASKS if t.get("attention") and t is not task]
        if task and args.workers and WORKERS[0].get("attention"):
            waiting.insert(0, MAIN)
        dots = []
        for name in waiting:
            right += f" · {name} "
            dots.append(len(right))
            right += icons["attention"]
        right += " "
        if len(left) + 1 + len(right) <= cols:
            start = cols - len(right)
            c.put(0, start, right, bar)
            for dot in dots:
                c.put(0, start + dot, icons["attention"], sgr(ATTENTION, pal.bar_bg))
    # The bar: a tab for each workspace that fits, with a dim separator
    # between them, the current one bold and the others dim, and an
    # attention icon after any workspace waiting for an approval. The current
    # workspace is the first.
    waiting = any(p.get("attention") for p in panes) or any(t.get("attention") for t in TASKS)
    tabs = [("hypercube", waiting), ("api-server", True), ("docs", False)]
    c.put(rows - 1, 0, " " * cols, bar)
    col = 1
    for i, (name, attention) in enumerate(tabs):
        bold = BOLD if i == 0 else ""
        sep = " · " if i else ""
        dot = f" {icons['attention']}" if attention else ""
        text = f"{i} {name}"
        if col + len(sep) + len(text) + len(dot) > cols:
            break
        c.put(rows - 1, col, sep, sgr(mix(pal.bar_fg, pal.bar_bg, 0.55), pal.bar_bg))
        col += len(sep)
        dim = sgr(mix(pal.bar_fg, pal.bar_bg, 0.4), pal.bar_bg)
        c.put(rows - 1, col, text, bar + bold if i == 0 else dim)
        col += len(text)
        if attention:
            c.put(rows - 1, col, dot, sgr(ATTENTION, pal.bar_bg) + bold)
            col += len(dot)
    return c.render(not args.no_border)


# The workspaces in number order, the first the current one: each one's
# name, checkout, accent in ACCENTS, whether its host runs, its main team's
# agents, and its tasks, each with its team's agents. The current one's
# checkout is --path, its accent --accent, and its teams those of WORKERS
# and TASKS.
WORKSPACES = [
    dict(name="hypercube"),
    dict(
        name="api-server",
        path="~/src/api-server",
        accent=0,
        running=True,
        agents=[
            dict(name="orchestrator", busy=True),
            dict(name="worker-1", busy=True, attention=True),
            dict(name="worker-2", busy=False),
        ],
        tasks=[dict(name="deps", state="idle", last="2h ago", agents=[dict(name="deps", busy=False)])],
    ),
    dict(
        name="docs",
        path="~/src/docs",
        accent=4,
        running=False,
        agents=[dict(name="orchestrator"), dict(name="worker-1")],
        tasks=[],
    ),
]


def agent_marks(icons, pane):
    """An agent's mark at the right of its sidebar row, the most urgent of:
    failed or exited, waiting for an approval, and busy. An idle agent has
    none."""
    if pane.get("failed"):
        return [(icons["task_failed"], ERROR)]
    if pane.get("status") is not None:
        return [(icons["exited"].format(pane["status"]), ERROR)]
    if pane.get("attention"):
        return [(icons["attention"], ATTENTION)]
    if pane.get("busy"):
        return [(icons["activity"], RUNNING)]
    return []


# The sidebar's columns right of its marks. A Nerd Font icon followed by a
# space draws across both cells, so the last mark needs two to clear the edge.
PAD = 2


def draw_sidebar(c, pal, icons, args, panes, task):
    """The sidebar, in place of the header, the bar, the workspace overlay,
    and the team list: every workspace in number order with its checkout,
    its main team, and its scheduled tasks' teams under "Scheduled", each
    team with its agents, and a row to start a workspace. The current
    workspace, the viewed team, and the focused agent are bold. With `--sidebar-focus` the `--selected`th selectable row is
    highlighted and the last row shows its keys."""
    width, rows, bg = args.sidebar, args.rows, pal.bar_bg
    text = sgr(pal.bar_fg, bg)
    faint = sgr(mix(pal.bar_fg, bg, 0.45), bg)
    for row in range(rows):
        c.put(row, 0, " " * width, text)
    # Each row: its indent, label, marks, style, whether it is bold, whether
    # it is selectable, and the color of the label's first word, if any.
    items = []

    def add(indent, label, marks=(), style=text, bold=False, select=True, lead=None):
        items.append(dict(indent=indent, label=label, marks=list(marks), style=style, bold=bold, select=select, lead=lead))

    def add_team(name, team_panes, style, viewed):
        """A team, its orchestrator under it, and its workers under the
        orchestrator."""
        add(3, name, style=style, bold=viewed)
        for i, p in enumerate(team_panes):
            add(7 if i else 5, p["name"], agent_marks(icons, p), style=style, bold=viewed and i == args.focus)

    def add_workspace(n, name, accent, path, main, tasks, running, current):
        style = text if running else faint
        add(1, f"{n} {name}", style=style, bold=current, lead=accent)
        room = width - 3 - PAD
        add(3, path if len(path) <= room else "…" + path[-(room - 1) :], style=faint, select=False)
        add_team(MAIN, main, style, current and task is None)
        if tasks:
            add(3, "Scheduled", style=faint, select=False)
        for t, agents in tasks:
            add_team(t["name"], agents, style, current and t is task)
        add(0, "", select=False)

    for n, ws in enumerate(WORKSPACES):
        if n == 0:
            main = team(icons, args, None)
            tasks = [(t, team(icons, args, t)) for t in TASKS]
            add_workspace(n, ws["name"], pal.line, args.path, main, tasks, True, True)
        else:
            tasks = [(t, t["agents"]) for t in ws["tasks"]]
            accent = Palette(ws["accent"]).line
            add_workspace(n, ws["name"], accent, ws["path"], ws["agents"], tasks, ws["running"], False)
    add(1, "+ workspace", style=faint)

    selectable = 0
    for row, item in enumerate(items[: rows - 1]):
        selected = args.sidebar_focus and item["select"] and selectable == args.selected
        selectable += item["select"]
        row_bg = pal.selected_bg if selected else bg
        style = sgr(pal.selected_fg, row_bg) if selected else item["style"]
        if selected:
            c.put(row, 0, " " * width, style)
        marks = " ".join(m for m, _ in item["marks"])
        room = width - item["indent"] - PAD - (len(marks) + 1 if marks else 0)
        label = item["label"]
        if len(label) > room:
            label = label[: max(room - 1, 0)] + "…"
        c.put(row, item["indent"], label, style + (BOLD if item["bold"] else ""))
        if item["lead"]:
            first = label.split(" ")[0]
            c.put(row, item["indent"], first, sgr(item["lead"], row_bg) + BOLD)
        col = width - PAD - len(marks)
        for mark, color in item["marks"]:
            c.put(row, col, mark, sgr(color, row_bg))
            col += len(mark) + 1
    hint = " ⏎ open  n new  x del" if args.sidebar_focus else " ^b ? keys"
    c.put(rows - 1, 0, hint[:width], faint)


# Each state's icon in ICONS.
STATE_ICONS = {"running": "running", "idle": "idle", "stopped": "stopped", "failed": "task_failed"}


def list_row(c, pal, icons, args, row, index, text, name, state, current, attention=False):
    """Screen row `row` of the overlay or the team list, the `index`th of its
    selectable rows: the state's icon in its color, then `text`, which starts
    with the name, bold on the current row, then an attention icon."""
    selected = index == args.selected
    bg = pal.selected_bg if selected else None
    paint = lambda color: sgr(color, bg) if bg else fg(color)
    base = sgr(pal.selected_fg, bg) if bg else RESET
    line = f" {icons[STATE_ICONS[state]]} {text}"
    if attention:
        line += f" {icons['attention']}"
    c.put(row, 0, line.ljust(args.cols) if selected else line, base)
    c.put(row, 1, icons[STATE_ICONS[state]], paint(STATES[state]))
    if current:
        c.put(row, 3, name, base + BOLD)
    if attention:
        c.put(row, len(line) - len(icons["attention"]), icons["attention"], paint(ATTENTION))


def draw_overlay(c, pal, icons, args):
    """The workspace overlay. Mirrors Overlay::draw. The agent counts are the
    main team's, and the task counts follow them while a workspace has tasks."""
    cols, rows = args.cols, args.rows
    bar = sgr(pal.bar_fg, pal.bar_bg)
    c.put(0, 0, " Workspaces".ljust(cols), bar)
    running_tasks = sum(t["state"] == "running" for t in TASKS)
    workspaces = [
        ("hypercube", True, 5, 2, len(TASKS), running_tasks, "now"),
        ("api-server", True, 3, 0, 2, 1, "4m ago"),
        ("docs", False, 1, 0, 0, 0, "2d ago"),
    ]
    counts = []
    for _, _, agents, busy, tasks, running_tasks, _ in workspaces:
        noun = "agent" if agents == 1 else "agents"
        count = f"{agents} {noun}, {busy} busy"
        if tasks:
            noun = "task" if tasks == 1 else "tasks"
            count += f"  {icon(icons, 'tasks', f'{tasks} {noun}, {running_tasks} running')}"
        counts.append(count)
    width = max(len(w[0]) for w in workspaces)
    count_width = max(len(c) for c in counts)
    for i, (name, running, _, _, _, _, active) in enumerate(workspaces):
        text = f"{name.ljust(width)}  {counts[i].ljust(count_width)}  {active}"
        list_row(c, pal, icons, args, 1 + i, i, text, name, "running" if running else "stopped", i == 0)
    c.put(rows - 1, 0, " ↑↓ move  enter switch  x stop  d detach  esc close", HELP)
    return c.render(not args.no_border)


def draw_teams(c, pal, icons, args):
    """The team list Ctrl-b t opens: the main team, then under "Scheduled" the
    team of each task, with its schedule and last run. The viewed team's name
    is bold. Mirrors the planned TaskList::draw, in the overlay's style."""
    cols, rows = args.cols, args.rows
    bar = sgr(pal.bar_fg, pal.bar_bg)
    c.put(0, 0, " Teams".ljust(cols), bar)
    main = dict(name=MAIN, schedule="", state="running", workers=args.workers, last="")
    width = max(len(t["name"]) for t in [main] + TASKS)
    schedule_width = max(len(t["schedule"]) for t in TASKS)

    def agents(t):
        n = t["workers"] + 1
        return f"{n} agent" if n == 1 else f"{n} agents"

    attention = args.workers and WORKERS[0].get("attention")
    text = f"{MAIN.ljust(width)}  {agents(main)}"
    list_row(c, pal, icons, args, 1, 0, text, MAIN, "running", args.view == 0, attention)
    c.put(3, 1, "Scheduled", HELP)
    for i, t in enumerate(TASKS, 1):
        text = f"{t['name'].ljust(width)}  {t['schedule'].ljust(schedule_width)}  {agents(t).ljust(8)}  {t['last']}"
        list_row(c, pal, icons, args, 3 + i, i, text, t["name"], t["state"], args.view == i, t.get("attention"))
    c.put(rows - 1, 0, " ↑↓ move  enter view  esc close", HELP)
    return c.render(not args.no_border)


def main():
    p = argparse.ArgumentParser(description=(__doc__ or "").split("\n")[0])
    p.add_argument("--cols", type=int, default=100)
    p.add_argument("--rows", type=int, default=30)
    p.add_argument("--workers", type=int, default=4)
    p.add_argument("--focus", type=int, default=0, help="focused pane index; 0 is the orchestrator")
    p.add_argument("--zoom", type=int, help="zoomed pane index")
    p.add_argument("--path", default="~/projects/hypercube", help="workspace path for the header")
    p.add_argument("--accent", type=int, default=len(ACCENTS) - 1, help="the workspace's accent, numbered in ACCENTS")
    p.add_argument("--icons", choices=ICONS, default="plain", help="the icon set")
    p.add_argument("--overlay", action="store_true", help="draw the workspace overlay")
    p.add_argument("--selected", type=int, default=1, help="the overlay's or team list's selected row")
    p.add_argument("--view", type=int, default=0, help="the viewed team: 0 is the main team, n the nth of TASKS")
    p.add_argument("--teams", action="store_true", help="draw the team list")
    p.add_argument("--no-tasks", action="store_true", help="draw the header of a workspace without tasks")
    p.add_argument(
        "--sidebar",
        type=int,
        nargs="?",
        const=30,
        default=0,
        metavar="WIDTH",
        help="draw the sidebar, 30 columns unless given, in place of the header and bar",
    )
    p.add_argument("--sidebar-focus", action="store_true", help="draw the sidebar with keyboard focus")
    p.add_argument("--no-border", action="store_true", help="omit the gray canvas border")
    print(draw(p.parse_args()))


if __name__ == "__main__":
    main()
