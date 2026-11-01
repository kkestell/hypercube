// Package store keeps workspaces, sessions, agents, and queued messages in one
// SQLite database that every Hypercube process opens.
package store

import (
	"cmp"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"

	_ "modernc.org/sqlite"
)

const (
	Lead   = "lead"
	Worker = "worker"
	Busy   = "busy"
	Idle   = "idle"
	Exited = "exited"
)

// Untitled is what a session shows until its first prompt titles it.
const Untitled = "New Session"

// Workspace is a folder that holds sessions. NoGit is set for a folder that is
// not a Git checkout; its workers work directly in existing directories.
type Workspace struct {
	ID      string
	Path    string
	NoGit   bool
	Created int64
}

// Session is a lead and its workers. Running is what the user last chose.
// AwaitingUser holds queued messages until the user prompts the session, and
// Unseen marks a lead reply the user has not seen.
type Session struct {
	ID           string
	Workspace    string
	Title        string
	Running      bool
	AwaitingUser bool
	Unseen       bool
	Created      int64
}

type Agent struct {
	ID           string
	Session      string
	Name         string
	Role         string
	Harness      string
	Model        string
	Effort       string
	Dir          string
	Branch       string
	Conversation string
	Pane         string
	Activity     string
	Attention    bool
	Waiter       int64
	Launched     int64
	QuickExits   int
	// Status is the worker's own account of what it is doing, reported at
	// StatusAt in Unix milliseconds.
	Status   string
	StatusAt int64
	Usage    Usage
	Created  int64
}

// Usage is what the agent's conversation has used: the tokens of its latest
// request, its model's context window, and its cost in US dollars. A harness
// that reports no cost leaves Cost zero.
type Usage struct {
	Context int     `json:"context,omitempty"`
	Window  int     `json:"window,omitempty"`
	Cost    float64 `json:"cost,omitempty"`
}

type Message struct {
	ID   int64
	From string
	To   string
	Text string
}

// Address names an agent across sessions. Session IDs and agent names cannot
// contain a slash, so the address is unambiguous.
func (a Agent) Address() string {
	return a.Session + "/" + a.Name
}

// Dir is the directory that holds the database, locks, and plugin files.
func Dir() string {
	base := os.Getenv("XDG_STATE_HOME")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(base, "hypercube")
}

// Path is the database file.
func Path() string {
	return filepath.Join(Dir(), "hypercube.db")
}

// NewID returns a short random identifier.
func NewID() string {
	b := make([]byte, 4)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func now() int64 { return time.Now().UnixMilli() }

const schema = `
CREATE TABLE IF NOT EXISTS workspaces (
  id TEXT PRIMARY KEY, path TEXT NOT NULL UNIQUE, no_git INTEGER NOT NULL,
  created INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS sessions (
  id TEXT PRIMARY KEY,
  workspace TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  title TEXT NOT NULL DEFAULT '', running INTEGER NOT NULL,
  awaiting_user INTEGER NOT NULL DEFAULT 0, unseen INTEGER NOT NULL DEFAULT 0,
  created INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS agents (
  id TEXT PRIMARY KEY,
  session TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
  name TEXT NOT NULL, role TEXT NOT NULL, harness TEXT NOT NULL,
  model TEXT NOT NULL, effort TEXT NOT NULL, dir TEXT NOT NULL,
  branch TEXT NOT NULL DEFAULT '', conversation TEXT NOT NULL DEFAULT '',
  pane TEXT NOT NULL DEFAULT '', activity TEXT NOT NULL,
  attention INTEGER NOT NULL DEFAULT 0, waiter INTEGER NOT NULL DEFAULT 0,
  launched INTEGER NOT NULL DEFAULT 0, quick_exits INTEGER NOT NULL DEFAULT 0,
  status TEXT NOT NULL DEFAULT '', status_at INTEGER NOT NULL DEFAULT 0,
  context INTEGER NOT NULL DEFAULT 0, window INTEGER NOT NULL DEFAULT 0,
  cost REAL NOT NULL DEFAULT 0, created INTEGER NOT NULL,
  UNIQUE (session, name));
CREATE TABLE IF NOT EXISTS messages (
  id INTEGER PRIMARY KEY,
  sender TEXT NOT NULL,
  recipient TEXT NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
  text TEXT NOT NULL);
`

// DB is the open database.
type DB struct{ sql *sql.DB }

// Open opens the database at Path, creating it and its tables if they are
// missing. Writes take the database lock when they begin and wait up to five
// seconds for it, so concurrent hooks wait instead of failing.
func Open() (*DB, error) {
	if err := os.MkdirAll(Dir(), 0o755); err != nil {
		return nil, err
	}
	params := url.Values{"_pragma": {"busy_timeout(5000)", "journal_mode(WAL)", "foreign_keys(1)"}, "_txlock": {"immediate"}}
	db, err := sql.Open("sqlite", Path()+"?"+params.Encode())
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("%s: %w", Path(), err)
	}
	return &DB{db}, nil
}

func (d *DB) Close() error { return d.sql.Close() }

// Tx runs fn in a write transaction and commits it when fn returns nil.
func (d *DB) Tx(fn func(*Tx) error) error {
	return d.run(false, fn)
}

// Read runs fn in a read transaction, which sees one consistent state and
// takes no write lock.
func (d *DB) Read(fn func(*Tx) error) error {
	return d.run(true, fn)
}

func (d *DB) run(readOnly bool, fn func(*Tx) error) error {
	tx, err := d.sql.BeginTx(context.Background(), &sql.TxOptions{ReadOnly: readOnly})
	if err != nil {
		return err
	}
	if err := fn(&Tx{tx}); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}

// Watch calls changed whenever another connection commits a change, checking
// every 250 milliseconds until ctx ends.
func (d *DB) Watch(ctx context.Context, changed func()) error {
	conn, err := d.sql.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	var last int64 = -1
	tick := time.NewTicker(250 * time.Millisecond)
	defer tick.Stop()
	for {
		var version int64
		if err := conn.QueryRowContext(ctx, "PRAGMA data_version").Scan(&version); err != nil {
			return err
		}
		if version != last {
			last = version
			changed()
		}
		select {
		case <-ctx.Done():
			return nil
		case <-tick.C:
		}
	}
}

// Lock holds the session's lifecycle lock until the file is closed. Lifecycle
// operations hold it instead of a database transaction, so a slow launch never
// keeps hooks waiting.
func Lock(session string) (*os.File, error) {
	path := filepath.Join(Dir(), "locks", session+".lock")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}

// Tx reads and writes within one transaction.
type Tx struct{ tx *sql.Tx }

func (t *Tx) exec(query string, args ...any) error {
	_, err := t.tx.Exec(query, args...)
	return err
}

func (t *Tx) AddWorkspace(w Workspace) error {
	return t.exec(`INSERT INTO workspaces (id, path, no_git, created) VALUES (?, ?, ?, ?)`,
		w.ID, w.Path, w.NoGit, cmp.Or(w.Created, now()))
}

const workspaceColumns = `id, path, no_git, created`

func scanWorkspace(row interface{ Scan(...any) error }) (Workspace, error) {
	var w Workspace
	err := row.Scan(&w.ID, &w.Path, &w.NoGit, &w.Created)
	return w, err
}

func (t *Tx) Workspace(id string) (Workspace, error) {
	w, err := scanWorkspace(t.tx.QueryRow(`SELECT `+workspaceColumns+` FROM workspaces WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		err = fmt.Errorf("no workspace %q", id)
	}
	return w, err
}

func (t *Tx) Workspaces() ([]Workspace, error) {
	rows, err := t.tx.Query(`SELECT ` + workspaceColumns + ` FROM workspaces ORDER BY created, rowid`)
	if err != nil {
		return nil, err
	}
	return collect(rows, scanWorkspace)
}

func (t *Tx) DeleteWorkspace(id string) error {
	return t.exec(`DELETE FROM workspaces WHERE id = ?`, id)
}

func (t *Tx) AddSession(s Session) error {
	return t.exec(`INSERT INTO sessions (id, workspace, title, running, awaiting_user, unseen, created) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		s.ID, s.Workspace, s.Title, s.Running, s.AwaitingUser, s.Unseen, cmp.Or(s.Created, now()))
}

func (t *Tx) SaveSession(s Session) error {
	return t.exec(`UPDATE sessions SET title = ?, running = ?, awaiting_user = ?, unseen = ? WHERE id = ?`,
		s.Title, s.Running, s.AwaitingUser, s.Unseen, s.ID)
}

const sessionColumns = `id, workspace, title, running, awaiting_user, unseen, created`

func scanSession(row interface{ Scan(...any) error }) (Session, error) {
	var s Session
	err := row.Scan(&s.ID, &s.Workspace, &s.Title, &s.Running, &s.AwaitingUser, &s.Unseen, &s.Created)
	return s, err
}

func (t *Tx) Session(id string) (Session, error) {
	s, err := scanSession(t.tx.QueryRow(`SELECT `+sessionColumns+` FROM sessions WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		err = fmt.Errorf("no session %q", id)
	}
	return s, err
}

// Sessions returns the workspace's sessions in creation order, or every
// session when workspace is empty.
func (t *Tx) Sessions(workspace string) ([]Session, error) {
	rows, err := t.tx.Query(`SELECT `+sessionColumns+` FROM sessions WHERE ? = '' OR workspace = ? ORDER BY created, rowid`, workspace, workspace)
	if err != nil {
		return nil, err
	}
	return collect(rows, scanSession)
}

func (t *Tx) DeleteSession(id string) error {
	return t.exec(`DELETE FROM sessions WHERE id = ?`, id)
}

const agentColumns = `id, session, name, role, harness, model, effort, dir, branch, conversation, pane,
  activity, attention, waiter, launched, quick_exits, status, status_at, context, window, cost, created`

func scanAgent(row interface{ Scan(...any) error }) (Agent, error) {
	var a Agent
	err := row.Scan(&a.ID, &a.Session, &a.Name, &a.Role, &a.Harness, &a.Model, &a.Effort, &a.Dir, &a.Branch,
		&a.Conversation, &a.Pane, &a.Activity, &a.Attention, &a.Waiter, &a.Launched, &a.QuickExits,
		&a.Status, &a.StatusAt, &a.Usage.Context, &a.Usage.Window, &a.Usage.Cost, &a.Created)
	return a, err
}

func (t *Tx) AddAgent(a Agent) error {
	return t.exec(`INSERT INTO agents (`+agentColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.ID, a.Session, a.Name, a.Role, a.Harness, a.Model, a.Effort, a.Dir, a.Branch, a.Conversation, a.Pane,
		a.Activity, a.Attention, a.Waiter, a.Launched, a.QuickExits, a.Status, a.StatusAt,
		a.Usage.Context, a.Usage.Window, a.Usage.Cost, cmp.Or(a.Created, now()))
}

// SaveAgent writes every field but the agent's identity and session.
func (t *Tx) SaveAgent(a Agent) error {
	return t.exec(`UPDATE agents SET harness = ?, model = ?, effort = ?, dir = ?, branch = ?, conversation = ?,
  pane = ?, activity = ?, attention = ?, waiter = ?, launched = ?, quick_exits = ?, status = ?, status_at = ?,
  context = ?, window = ?, cost = ? WHERE id = ?`,
		a.Harness, a.Model, a.Effort, a.Dir, a.Branch, a.Conversation, a.Pane, a.Activity, a.Attention, a.Waiter,
		a.Launched, a.QuickExits, a.Status, a.StatusAt, a.Usage.Context, a.Usage.Window, a.Usage.Cost, a.ID)
}

func (t *Tx) Agent(id string) (Agent, error) {
	a, err := scanAgent(t.tx.QueryRow(`SELECT `+agentColumns+` FROM agents WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		err = fmt.Errorf("no agent %q", id)
	}
	return a, err
}

// Agents returns the session's agents in creation order, the lead first.
func (t *Tx) Agents(session string) ([]Agent, error) {
	rows, err := t.tx.Query(`SELECT `+agentColumns+` FROM agents WHERE session = ? ORDER BY role != 'lead', created, rowid`, session)
	if err != nil {
		return nil, err
	}
	return collect(rows, scanAgent)
}

// Lead returns the session's lead.
func (t *Tx) Lead(session string) (Agent, error) {
	return t.Find(session, Lead)
}

// Find resolves an address, or a bare name within the given session.
func (t *Tx) Find(session, address string) (Agent, error) {
	s, name, qualified := strings.Cut(address, "/")
	if qualified {
		session = s
	} else {
		name = address
	}
	a, err := scanAgent(t.tx.QueryRow(`SELECT `+agentColumns+` FROM agents WHERE session = ? AND name = ?`, session, name))
	if errors.Is(err, sql.ErrNoRows) {
		err = fmt.Errorf("no agent %q", address)
	}
	return a, err
}

func (t *Tx) DeleteAgent(id string) error {
	return t.exec(`DELETE FROM agents WHERE id = ?`, id)
}

// Queue adds a message for its recipient.
func (t *Tx) Queue(m Message) error {
	return t.exec(`INSERT INTO messages (sender, recipient, text) VALUES (?, ?, ?)`, m.From, m.To, m.Text)
}

// Queued returns the messages waiting for an agent, oldest first.
func (t *Tx) Queued(id string) ([]Message, error) {
	rows, err := t.tx.Query(`SELECT id, sender, recipient, text FROM messages WHERE recipient = ? ORDER BY id`, id)
	if err != nil {
		return nil, err
	}
	return collect(rows, func(r interface{ Scan(...any) error }) (Message, error) {
		var m Message
		err := r.Scan(&m.ID, &m.From, &m.To, &m.Text)
		return m, err
	})
}

// Take removes and returns the messages queued for an agent.
func (t *Tx) Take(id string) ([]Message, error) {
	messages, err := t.Queued(id)
	if err != nil || len(messages) == 0 {
		return nil, err
	}
	return messages, t.exec(`DELETE FROM messages WHERE recipient = ? AND id <= ?`, id, messages[len(messages)-1].ID)
}

func collect[T any](rows *sql.Rows, scan func(interface{ Scan(...any) error }) (T, error)) ([]T, error) {
	defer rows.Close()
	var list []T
	for rows.Next() {
		v, err := scan(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, v)
	}
	return list, rows.Err()
}

// ShownWorkspace, ShownSession, and ShownAgent are what the browser view shows.
type ShownWorkspace struct {
	ID       string         `json:"id"`
	Path     string         `json:"path"`
	Name     string         `json:"name"`
	Sessions []ShownSession `json:"sessions"`
}

type ShownSession struct {
	ID      string       `json:"id"`
	Title   string       `json:"title"`
	Running bool         `json:"running"`
	Unseen  bool         `json:"unseen"`
	Agents  []ShownAgent `json:"agents"`
}

type ShownAgent struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Role      string `json:"role"`
	Harness   string `json:"harness"`
	Model     string `json:"model"`
	Effort    string `json:"effort"`
	Dir       string `json:"dir"`
	Activity  string `json:"activity"`
	Attention bool   `json:"attention"`
	Status    string `json:"status"`
	StatusAt  int64  `json:"statusAt"`
	Usage     Usage  `json:"usage"`
}

// Snapshot reads everything the view shows in one transaction: workspaces by
// folder name, each with its sessions in creation order.
func Snapshot(db *DB) ([]ShownWorkspace, error) {
	list := []ShownWorkspace{}
	err := db.Read(func(tx *Tx) error {
		workspaces, err := tx.Workspaces()
		if err != nil {
			return err
		}
		for _, w := range workspaces {
			shown := ShownWorkspace{ID: w.ID, Path: w.Path, Name: filepath.Base(w.Path), Sessions: []ShownSession{}}
			sessions, err := tx.Sessions(w.ID)
			if err != nil {
				return err
			}
			for _, s := range sessions {
				agents, err := tx.Agents(s.ID)
				if err != nil {
					return err
				}
				session := ShownSession{ID: s.ID, Title: cmp.Or(s.Title, Untitled), Running: s.Running, Unseen: s.Unseen, Agents: []ShownAgent{}}
				for _, a := range agents {
					session.Agents = append(session.Agents, ShownAgent{a.ID, a.Name, a.Role, a.Harness, a.Model, a.Effort,
						a.Dir, a.Activity, a.Attention, a.Status, a.StatusAt, a.Usage})
				}
				shown.Sessions = append(shown.Sessions, session)
			}
			list = append(list, shown)
		}
		return nil
	})
	slices.SortStableFunc(list, func(a, b ShownWorkspace) int { return cmp.Compare(a.Name, b.Name) })
	return list, err
}
