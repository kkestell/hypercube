// Package store keeps workspaces, sessions, queued messages, and
// transcripts in one SQLite database that the app process owns.
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
	"time"

	"github.com/kkestell/hypercube/internal/harness"

	_ "modernc.org/sqlite"
)

// ErrNotFound is returned when a lookup matches nothing.
var ErrNotFound = errors.New("not found")

const (
	Busy = "busy"
	Idle = "idle"
	// Exited also covers startup before the plugin accepts the start command;
	// it does not establish whether a process exists.
	Exited = "exited"
)

// Workspace is a folder that holds sessions. NoGit is set for a folder that is
// not a Git checkout; its child sessions work directly in existing directories.
type Workspace struct {
	ID      string
	Path    string
	NoGit   bool
	Created int64
}

// Session has one agent. Family state belongs only to a top-level session.
type Session struct {
	ID           string
	Workspace    string
	Parent       string
	Name         string
	Title        string
	Ended        bool
	Unseen       bool
	Harness      string
	Model        string
	Effort       string
	Dir          string
	Branch       string
	Conversation string
	// Fork is the native source to copy until Conversation names the child's copy.
	Fork string
	// Activity is Busy, Idle, or Exited, separate from the saved Ended flag.
	Activity  string
	Attention bool
	// InputPending is recorded input that has not yet begun work.
	InputPending bool
	Usage        harness.Usage
	Created      int64
	Launch       string
	Position     int64
	Ready        int64
	Quick        int
	// Base is the commit a top-level worktree session's branch started from.
	// It is empty for every other session.
	Base string
	// Tip is the tip of an archived top-level worktree session's branch, which
	// unarchiving restores. It is empty for every other session.
	Tip string
	// Archived is when the session was archived, in Unix milliseconds. It is
	// zero for an active session and for one archived before it was recorded.
	Archived int64
}

// Message is a queued message. From is empty for Hypercube's own messages.
type Message struct {
	ID     int64
	From   string
	To     string
	Text   string
	Report bool
}

// Dir is the directory that holds the database and plugin files.
func Dir() string {
	base := os.Getenv("XDG_STATE_HOME")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(base, "hypercube")
}

// dbPath is the database file.
func dbPath() string {
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
CREATE TABLE IF NOT EXISTS github_accounts (
  workspace TEXT PRIMARY KEY REFERENCES workspaces(id) ON DELETE CASCADE,
  account TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS sessions (
  id TEXT PRIMARY KEY,
  workspace TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  parent TEXT REFERENCES sessions(id) ON DELETE CASCADE,
  name TEXT NOT NULL DEFAULT '', title TEXT NOT NULL DEFAULT '',
  ended INTEGER NOT NULL DEFAULT 0, unseen INTEGER NOT NULL DEFAULT 0,
  harness TEXT NOT NULL, model TEXT NOT NULL, effort TEXT NOT NULL,
  dir TEXT NOT NULL, branch TEXT NOT NULL DEFAULT '',
  conversation TEXT NOT NULL DEFAULT '', fork TEXT NOT NULL DEFAULT '', activity TEXT NOT NULL,
  attention INTEGER NOT NULL DEFAULT 0, input_pending INTEGER NOT NULL DEFAULT 0,
  context INTEGER NOT NULL DEFAULT 0, window INTEGER NOT NULL DEFAULT 0,
  cost REAL NOT NULL DEFAULT 0, created INTEGER NOT NULL,
  launch TEXT NOT NULL DEFAULT '', position INTEGER NOT NULL DEFAULT 0,
  ready INTEGER NOT NULL DEFAULT 0, quick INTEGER NOT NULL DEFAULT 0,
  base TEXT NOT NULL DEFAULT '', tip TEXT NOT NULL DEFAULT '',
  archived INTEGER NOT NULL DEFAULT 0,
  -- Required inert field in the current schema; initialized to created.
  last_active INTEGER NOT NULL, UNIQUE (parent, name));
CREATE TABLE IF NOT EXISTS commands (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  session TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
  command TEXT NOT NULL, line TEXT NOT NULL, message INTEGER, seq INTEGER, item TEXT);
CREATE TABLE IF NOT EXISTS messages (
  id INTEGER PRIMARY KEY,
  sender TEXT NOT NULL,
  recipient TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
  text TEXT NOT NULL, report INTEGER NOT NULL DEFAULT 0);
CREATE TABLE IF NOT EXISTS transcript (
  session TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
  seq INTEGER NOT NULL, item TEXT NOT NULL, data TEXT NOT NULL,
  PRIMARY KEY (session, seq), UNIQUE (session, item));
CREATE INDEX IF NOT EXISTS transcript_root_text_search ON transcript (
  session, seq, lower(json_extract(data, '$.text')))
WHERE json_extract(data, '$.kind') IN ('user', 'text')
  AND json_extract(data, '$.parent') IS NULL;
CREATE INDEX IF NOT EXISTS transcript_message_text_search ON transcript (
  session, seq, lower(json_extract(data, '$.input.text')))
WHERE json_extract(data, '$.kind') = 'tool'
  AND json_extract(data, '$.server') = 'hypercube'
  AND json_extract(data, '$.tool') = 'send_message';
-- A grant with no session covers every session of its harness.
CREATE TABLE IF NOT EXISTS grants (
  session TEXT REFERENCES sessions(id) ON DELETE CASCADE,
  harness TEXT NOT NULL, tool TEXT NOT NULL);
CREATE UNIQUE INDEX IF NOT EXISTS grants_unique ON grants (COALESCE(session, ''), harness, tool);
CREATE TABLE IF NOT EXISTS limits (
  harness TEXT NOT NULL, minutes INTEGER NOT NULL, used INTEGER NOT NULL,
  resets INTEGER NOT NULL, at INTEGER NOT NULL, PRIMARY KEY (harness, minutes));
`

// DB is the open database.
type DB struct{ sql *sql.DB }

// Open opens the database in Dir, creating its tables if they are missing,
// adding the session columns earlier builds lacked, and moving model entries
// saved by earlier builds to config.json.
// Writes take the database lock when they begin and wait up to five seconds
// for it.
func Open() (*DB, error) {
	if err := os.MkdirAll(Dir(), 0o700); err != nil {
		return nil, err
	}
	// Also protect transcripts and plugin state in existing installations.
	if err := os.Chmod(Dir(), 0o700); err != nil {
		return nil, err
	}
	params := url.Values{"_pragma": {"busy_timeout(5000)", "journal_mode(WAL)", "foreign_keys(1)"}, "_txlock": {"immediate"}}
	db, err := sql.Open("sqlite", dbPath()+"?"+params.Encode())
	if err != nil {
		return nil, err
	}
	d := &DB{db}
	if err := d.Tx(func(tx *Tx) error {
		if err := tx.exec(schema); err != nil {
			return err
		}
		if err := tx.addColumn("tip", `TEXT NOT NULL DEFAULT ''`); err != nil {
			return err
		}
		return tx.addColumn("archived", `INTEGER NOT NULL DEFAULT 0`)
	}); err != nil {
		db.Close()
		return nil, fmt.Errorf("%s: %w", dbPath(), err)
	}
	if err := d.moveModelEntries(); err != nil {
		db.Close()
		return nil, fmt.Errorf("moving model entries to config.json: %w", err)
	}
	return d, nil
}

func (d *DB) Close() error { return d.sql.Close() }

// Tx runs fn in a write transaction and commits it when fn returns nil.
func (d *DB) Tx(fn func(*Tx) error) error {
	return d.run(context.Background(), false, fn)
}

// Read runs fn in a read transaction, which sees one consistent state and
// takes no write lock.
func (d *DB) Read(fn func(*Tx) error) error {
	return d.run(context.Background(), true, fn)
}

// ReadContext lets a request stop a long read when its client leaves.
func (d *DB) ReadContext(ctx context.Context, fn func(*Tx) error) error {
	return d.run(ctx, true, fn)
}

func (d *DB) run(ctx context.Context, readOnly bool, fn func(*Tx) error) error {
	tx, err := d.sql.BeginTx(ctx, &sql.TxOptions{ReadOnly: readOnly})
	if err != nil {
		return err
	}
	// Rollback after Commit does nothing; deferring it also ends the
	// transaction when fn panics.
	defer tx.Rollback()
	if err := fn(&Tx{tx}); err != nil {
		return err
	}
	return tx.Commit()
}

// Tx reads and writes within one transaction.
type Tx struct{ tx *sql.Tx }

func (t *Tx) exec(query string, args ...any) error {
	_, err := t.tx.Exec(query, args...)
	return err
}

// addColumn adds a column to the sessions table of a database created without
// it.
func (t *Tx) addColumn(name, definition string) error {
	var found bool
	if err := t.tx.QueryRow(`SELECT EXISTS (SELECT 1 FROM pragma_table_info('sessions') WHERE name = ?)`, name).Scan(&found); err != nil || found {
		return err
	}
	return t.exec(`ALTER TABLE sessions ADD COLUMN ` + name + ` ` + definition)
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
		err = fmt.Errorf("workspace %q: %w", id, ErrNotFound)
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

func (t *Tx) SetWorkspaceNoGit(id string, noGit bool) error {
	return t.exec(`UPDATE workspaces SET no_git = ? WHERE id = ?`, noGit, id)
}

func (t *Tx) DeleteWorkspace(id string) error {
	return t.exec(`DELETE FROM workspaces WHERE id = ?`, id)
}

// GitHubAccounts returns the GitHub account the user chose for each workspace,
// by workspace ID.
func (t *Tx) GitHubAccounts() (map[string]string, error) {
	rows, err := t.tx.Query(`SELECT workspace, account FROM github_accounts`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	accounts := map[string]string{}
	for rows.Next() {
		var workspace, account string
		if err := rows.Scan(&workspace, &account); err != nil {
			return nil, err
		}
		accounts[workspace] = account
	}
	return accounts, rows.Err()
}

// SetGitHubAccount saves the user's choice of GitHub account for a workspace.
func (t *Tx) SetGitHubAccount(workspace, account string) error {
	return t.exec(`INSERT INTO github_accounts (workspace, account) VALUES (?, ?)
 ON CONFLICT (workspace) DO UPDATE SET account = excluded.account`, workspace, account)
}

func (t *Tx) AddSession(s Session) error {
	created := cmp.Or(s.Created, now())
	return t.exec(`INSERT INTO sessions (id, workspace, parent, name, title, ended,
 unseen, harness, model, effort, dir, branch, conversation,
 activity, attention, context, window, cost, created, last_active, launch, position, ready, quick, base, tip, fork, input_pending)
 VALUES (?, ?, NULLIF(?, ''), ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		s.ID, s.Workspace, s.Parent, s.Name, s.Title, s.Ended,
		s.Unseen, s.Harness, s.Model, s.Effort, s.Dir, s.Branch,
		s.Conversation, s.Activity, s.Attention, s.Usage.Context, s.Usage.Window,
		s.Usage.Cost, created, created, s.Launch, s.Position, s.Ready, s.Quick, s.Base, s.Tip, s.Fork, s.InputPending)
}

// SaveSession writes every mutable field.
func (t *Tx) SaveSession(s Session) error {
	return t.exec(`UPDATE sessions SET title = ?, ended = ?, unseen = ?,
 harness = ?, model = ?, effort = ?, dir = ?, branch = ?, conversation = ?,
 activity = ?, attention = ?, context = ?, window = ?, cost = ?, launch = ?, position = ?, ready = ?, quick = ?, fork = ?, input_pending = ?, base = ?, tip = ?, archived = ? WHERE id = ?`,
		s.Title, s.Ended, s.Unseen, s.Harness, s.Model, s.Effort,
		s.Dir, s.Branch, s.Conversation, s.Activity, s.Attention, s.Usage.Context,
		s.Usage.Window, s.Usage.Cost, s.Launch, s.Position, s.Ready, s.Quick, s.Fork, s.InputPending, s.Base, s.Tip, s.Archived, s.ID)
}

const sessionColumns = `id, workspace, COALESCE(parent, ''), name, title, ended,
 unseen, harness, model, effort, dir, branch, conversation,
 activity, attention, context, window, cost, created, launch, position, ready, quick, base, tip, fork, input_pending, archived`

func scanSession(row interface{ Scan(...any) error }) (Session, error) {
	var s Session
	err := row.Scan(&s.ID, &s.Workspace, &s.Parent, &s.Name, &s.Title, &s.Ended,
		&s.Unseen, &s.Harness, &s.Model, &s.Effort, &s.Dir, &s.Branch,
		&s.Conversation, &s.Activity, &s.Attention, &s.Usage.Context, &s.Usage.Window,
		&s.Usage.Cost, &s.Created, &s.Launch, &s.Position, &s.Ready, &s.Quick, &s.Base, &s.Tip, &s.Fork, &s.InputPending, &s.Archived)
	return s, err
}

func (t *Tx) Session(id string) (Session, error) {
	s, err := scanSession(t.tx.QueryRow(`SELECT `+sessionColumns+` FROM sessions WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		err = fmt.Errorf("session %q: %w", id, ErrNotFound)
	}
	return s, err
}

// Sessions returns top-level sessions in creation order, in every workspace
// when workspace is empty.
func (t *Tx) Sessions(workspace string) ([]Session, error) {
	rows, err := t.tx.Query(`SELECT `+sessionColumns+` FROM sessions WHERE parent IS NULL AND (? = '' OR workspace = ?) ORDER BY created, rowid`, workspace, workspace)
	if err != nil {
		return nil, err
	}
	return collect(rows, scanSession)
}

func (t *Tx) Children(parent string) ([]Session, error) {
	rows, err := t.tx.Query(`SELECT `+sessionColumns+` FROM sessions WHERE parent = ? ORDER BY created, rowid`, parent)
	if err != nil {
		return nil, err
	}
	return collect(rows, scanSession)
}

func (t *Tx) Root(id string) (Session, error) {
	s, err := t.Session(id)
	if err != nil || s.Parent == "" {
		return s, err
	}
	return t.Session(s.Parent)
}

// FindSession is the messaging scope: a parent sees its children by name and
// other top-level sessions by ID; a child sees only its parent.
func (t *Tx) FindSession(caller Session, to string) (Session, error) {
	var s Session
	var err error
	if caller.Parent != "" {
		if to == caller.Parent {
			return t.Session(to)
		}
		return s, fmt.Errorf("session %q: %w", to, ErrNotFound)
	}
	s, err = scanSession(t.tx.QueryRow(`SELECT `+sessionColumns+` FROM sessions
 WHERE parent = ? AND name = ?`, caller.ID, to))
	if errors.Is(err, sql.ErrNoRows) {
		s, err = scanSession(t.tx.QueryRow(`SELECT `+sessionColumns+` FROM sessions
  WHERE parent IS NULL AND id = ?`, to))
	}
	if errors.Is(err, sql.ErrNoRows) {
		err = fmt.Errorf("session %q: %w", to, ErrNotFound)
	}
	return s, err
}

func (t *Tx) DeleteSession(id string) error {
	return t.exec(`DELETE FROM sessions WHERE id = ?`, id)
}

// Queue adds a message for its recipient.
func (t *Tx) Queue(m Message) error {
	return t.exec(`INSERT INTO messages (sender, recipient, text, report) VALUES (?, ?, ?, ?)`, m.From, m.To, m.Text, m.Report)
}

// Queued returns the messages waiting for a session, oldest first.
func (t *Tx) Queued(id string) ([]Message, error) {
	rows, err := t.tx.Query(`SELECT id, sender, recipient, text, report FROM messages WHERE recipient = ? ORDER BY id`, id)
	if err != nil {
		return nil, err
	}
	return collect(rows, func(r interface{ Scan(...any) error }) (Message, error) {
		var m Message
		err := r.Scan(&m.ID, &m.From, &m.To, &m.Text, &m.Report)
		return m, err
	})
}

// Dequeue removes a delivered message.
func (t *Tx) Dequeue(id int64) error {
	return t.exec(`DELETE FROM messages WHERE id = ?`, id)
}

// BatchReports replaces the first queued report with the delivery snapshot.
// Its caller records the input command in the same transaction.
func (t *Tx) BatchReports(ids []int64, text string) error {
	if err := t.exec(`UPDATE messages SET sender = '', text = ? WHERE id = ?`, text, ids[0]); err != nil {
		return err
	}
	for _, id := range ids[1:] {
		if err := t.Dequeue(id); err != nil {
			return err
		}
	}
	return nil
}

func (t *Tx) DropQueued(id string) error {
	return t.exec(`DELETE FROM messages WHERE recipient = ?`, id)
}

// Entry is one item of a session's transcript: its position, the item's ID, and
// the item as JSON.
type Entry struct {
	Seq  int64
	Item string
	Data []byte
}

// PutEntry saves a transcript item and returns its position. An item
// with the same ID as an earlier one replaces its data and keeps its position.
func (t *Tx) PutEntry(session string, e Entry) (int64, error) {
	var seq int64
	err := t.tx.QueryRow(`INSERT INTO transcript (session, seq, item, data) VALUES (?, ?, ?, ?)
  ON CONFLICT (session, item) DO UPDATE SET data = excluded.data RETURNING seq`, session, e.Seq, e.Item, string(e.Data)).Scan(&seq)
	return seq, err
}

// Entry returns the saved item with the ID, or false if there is none.
func (t *Tx) Entry(session, item string) (Entry, bool, error) {
	e := Entry{Item: item}
	var data string
	err := t.tx.QueryRow(`SELECT seq, data FROM transcript WHERE session = ? AND item = ?`, session, item).Scan(&e.Seq, &data)
	e.Data = []byte(data)
	if errors.Is(err, sql.ErrNoRows) {
		return e, false, nil
	}
	return e, err == nil, err
}

// Transcript returns the session's saved items in order.
func (t *Tx) Transcript(session string) ([]Entry, error) {
	rows, err := t.tx.Query(`SELECT seq, item, data FROM transcript WHERE session = ? ORDER BY seq`, session)
	if err != nil {
		return nil, err
	}
	return collect(rows, func(r interface{ Scan(...any) error }) (Entry, error) {
		var e Entry
		var data string
		err := r.Scan(&e.Seq, &e.Item, &data)
		e.Data = []byte(data)
		return e, err
	})
}

// CompactEntry holds only the fields used by read_session's text view.
type CompactEntry struct {
	Seq                         int64
	ID, Parent, Kind, Text      string
	From, Tool, Answer, State   string
	Automatic, Failed, HasFinal bool
}

// CompactEntries returns at most limit entries, newest first, before the
// exclusive position. A nonpositive before starts at the end of the transcript.
// Project in SQLite so tool output, images, and other unused data stay there.
func (t *Tx) CompactEntries(session string, before int64, limit int) ([]CompactEntry, error) {
	query := `SELECT seq, item,
 COALESCE(json_extract(data, '$.parent'), ''),
 COALESCE(json_extract(data, '$.kind'), ''),
 COALESCE(json_extract(data, '$.text'), ''),
 COALESCE(json_extract(data, '$.from'), ''),
 COALESCE(json_extract(data, '$.tool'), ''),
 COALESCE(json_extract(data, '$.answer'), ''),
 COALESCE(json_extract(data, '$.state'), ''),
 COALESCE(json_extract(data, '$.automatic'), 0),
 COALESCE(json_extract(data, '$.failed'), 0),
 COALESCE(json_array_length(data, '$.final'), 0) > 0
 FROM transcript WHERE session = ?`
	args := []any{session}
	if before > 0 {
		query += ` AND seq < ?`
		args = append(args, before)
	}
	query += ` ORDER BY seq DESC LIMIT ?`
	args = append(args, limit)
	rows, err := t.tx.Query(query, args...)
	if err != nil {
		return nil, err
	}
	return collect(rows, func(r interface{ Scan(...any) error }) (CompactEntry, error) {
		var e CompactEntry
		err := r.Scan(&e.Seq, &e.ID, &e.Parent, &e.Kind, &e.Text, &e.From,
			&e.Tool, &e.Answer, &e.State, &e.Automatic, &e.Failed, &e.HasFinal)
		return e, err
	})
}

// LastText returns the latest top-level text and its position, or "" and zero.
func (t *Tx) LastText(session string) (string, int64, error) {
	var text string
	var seq int64
	err := t.tx.QueryRow(`SELECT COALESCE(json_extract(data, '$.text'), ''), seq FROM transcript
  WHERE session = ? AND json_extract(data, '$.kind') = 'text' AND json_extract(data, '$.parent') IS NULL AND json_extract(data, '$.done') = 1
  ORDER BY seq DESC LIMIT 1`, session).Scan(&text, &seq)
	if errors.Is(err, sql.ErrNoRows) {
		return "", 0, nil
	}
	return text, seq, err
}

// LastSeq returns the largest saved or reserved transcript position, or zero.
func (t *Tx) LastSeq(session string) (int64, error) {
	var seq int64
	err := t.tx.QueryRow(`SELECT MAX(COALESCE((SELECT MAX(seq) FROM transcript WHERE session = ?), 0),
 COALESCE((SELECT MAX(seq) FROM commands WHERE session = ?), 0))`, session, session).Scan(&seq)
	return seq, err
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

// Command is an unanswered command for the session's current launch. Line is
// the exact JSON sent; Item and Seq reserve an accepted input's transcript entry.
type Command struct {
	ID                     int64
	Session, Command, Line string
	Message, Seq           int64
	Item                   []byte
}

// AddCommand records a command without its line and returns its ID, which the
// line carries. SetCommandLine saves the line.
func (t *Tx) AddCommand(c Command) (int64, error) {
	result, err := t.tx.Exec(`INSERT INTO commands (session, command, line, message, seq, item)
 VALUES (?, ?, '', NULLIF(?,0), NULLIF(?,0), ?)`, c.Session, c.Command, c.Message, c.Seq, string(c.Item))
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}
func (t *Tx) SetCommandLine(id int64, line string) error {
	return t.exec(`UPDATE commands SET line = ? WHERE id = ?`, line, id)
}
func (t *Tx) Commands(session string) ([]Command, error) {
	rows, err := t.tx.Query(`SELECT id, session, command, line, COALESCE(message,0), COALESCE(seq,0), COALESCE(item,'') FROM commands WHERE session = ? ORDER BY id`, session)
	if err != nil {
		return nil, err
	}
	return collect(rows, func(row interface{ Scan(...any) error }) (Command, error) {
		var c Command
		var item string
		err := row.Scan(&c.ID, &c.Session, &c.Command, &c.Line, &c.Message, &c.Seq, &item)
		c.Item = []byte(item)
		return c, err
	})
}
func (t *Tx) DeleteCommand(id int64) error { return t.exec(`DELETE FROM commands WHERE id = ?`, id) }
func (t *Tx) DeleteCommands(session string) error {
	return t.exec(`DELETE FROM commands WHERE session = ?`, session)
}
