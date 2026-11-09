package mcp

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/report"
	"github.com/kkestell/hypercube/internal/session"
	"github.com/kkestell/hypercube/internal/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// The ways create_session begins a child that has no accepted offer yet.
const (
	initiationOffer = "offer"
	initiationOwn   = "own_initiative"
	initiationAsked = "user_asked"
)

type createSessionArgs struct {
	Title      string `json:"title"`
	Initiation string `json:"initiation,omitempty"`
	OfferID    string `json:"offer_id,omitempty"`
	Name       string `json:"name,omitempty"`
	Harness    string `json:"harness,omitempty"`
	Model      string `json:"model,omitempty"`
	Effort     string `json:"effort,omitempty"`
	Dir        string `json:"dir,omitempty"`
	Message    string `json:"message,omitempty"`
	Fork       bool   `json:"fork,omitempty"`
}

type endSessionArgs struct {
	Name string `json:"name"`
}

const createDescription = `Start or suggest a child session.
- Requested delegation: when the user explicitly asks you to start another session or delegate work, pass initiation "user_asked" with the child's settings and brief in message. This starts it immediately; do not ask again for work the user has authorized.
- Offer: for a separate new task whose placement the user has not chosen, pass initiation "offer" and a short, meaningful title (48 characters or fewer) only. The user chooses New session, Do it here, or Dismiss. Dismiss closes the offer without authorizing work in any session. End your turn: the answer arrives as a message. Continuing an existing task, including corrections and small follow-ups, needs no offer.
- Accepted offer: when the answer is New session, pass its offer_id with the same title, a name, and a message that briefs the child. It starts with fresh context.
- Suggestion: for work you suggest yourself, pass initiation "own_initiative" with the child's settings to save a card the user can start or dismiss. This starts nothing until the user clicks Start.
The name is the child's address. It must use only lowercase letters, digits, and hyphens, start with a letter or digit, and be unique among your children (for example, lifecycle-terms). The message is queued for the child before it starts. ` + report.ChildSettings + ` harness, model, and effort override yours: another model starts at its default effort unless you pass one, and another harness needs a model. list_models lists the choices. Fork copies your native conversation, requires your harness, and works where you work: in your folder when you work in the original checkout, otherwise in a new worktree from your HEAD; a fork takes no dir.`

const dirDescription = " Without dir, a fresh child gets its own branch and worktree from HEAD. With dir, an existing directory given as an absolute path, a path starting with ~/, or a path relative to your directory, the child works directly in it and Hypercube creates no branch or worktree."

const noGitDirDescription = " A fresh child needs dir: an existing directory given as an absolute path, a path starting with ~/, or a path relative to your directory, where the child works directly."

// addChildTools adds the tools a top-level session uses to manage its
// children.
func addChildTools(server *mcp.Server, rt *session.Runtime, self string, w store.Workspace) {
	schema, err := jsonschema.For[createSessionArgs](nil)
	if err != nil {
		panic(err)
	}
	schema.Properties["initiation"].Enum = []any{initiationOffer, initiationOwn, initiationAsked}
	description := createDescription + dirDescription
	if w.NoGit {
		description = createDescription + noGitDirDescription
	}
	mcp.AddTool(server, &mcp.Tool{Name: "create_session", Description: description, InputSchema: schema}, result(func(_ context.Context, a createSessionArgs) (string, error) {
		return createSession(rt, self, a)
	}))
	mcp.AddTool(server, &mcp.Tool{
		Name:        "end_session",
		Description: "End a child session: its agent stops for good, and a child with a worktree also has its worktree and branch removed; Git refuses dirty worktrees and unmerged branches. The ended child stays listed for the user and cannot be messaged.",
	}, result(func(_ context.Context, a endSessionArgs) (string, error) { return endSession(rt, self, a.Name) }))
}

func createSession(rt *session.Runtime, self string, args createSessionArgs) (string, error) {
	switch {
	case args.Initiation == initiationOffer:
		if args.OfferID != "" || args.Name != "" || args.Harness != "" || args.Model != "" || args.Effort != "" || args.Dir != "" || args.Message != "" || args.Fork {
			return "", errors.New(`an offer takes only initiation "offer" and a title; supply the child's settings once the user chooses New session`)
		}
		id, err := rt.OfferChild(self, args.Title)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("Offered %q as offer %s. End your turn now; the user's answer arrives as a message.", args.Title, id), nil
	case args.OfferID != "":
		if args.Initiation != "" {
			return "", errors.New("an accepted offer takes offer_id without initiation")
		}
		options, err := childOptions(args)
		if err != nil {
			return "", err
		}
		child, err := rt.StartOffer(self, args.OfferID, options)
		if err != nil {
			return "", err
		}
		return childCreated(child), nil
	case args.Initiation == initiationAsked:
		if strings.TrimSpace(args.Message) == "" {
			return "", errors.New("a requested child session needs its brief in message")
		}
		options, err := childOptions(args)
		if err != nil {
			return "", err
		}
		child, err := rt.CreateChild(self, options)
		if err != nil {
			return "", err
		}
		return childCreated(child), nil
	case args.Initiation == initiationOwn:
		options, err := childOptions(args)
		if err != nil {
			return "", err
		}
		settings, err := rt.ProposeChild(self, options)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("Proposed %s with %s. The user can start it from the card; no child has started.", args.Name, report.Selection(settings.Harness, settings.Model, settings.Effort)), nil
	default:
		return "", errors.New(`pass initiation "user_asked", "offer", or "own_initiative", or the offer_id of an accepted offer`)
	}
}

func childCreated(child store.Session) string {
	settings := report.Selection(child.Harness, child.Model, child.Effort)
	if child.Branch == "" {
		return fmt.Sprintf("Created %s with %s in %s.", child.Name, settings, child.Dir)
	}
	return fmt.Sprintf("Created %s with %s on branch %s in %s.", child.Name, settings, child.Branch, child.Dir)
}

func childOptions(args createSessionArgs) (session.ChildOptions, error) {
	if args.Fork && args.Dir != "" {
		return session.ChildOptions{}, errors.New("a fork works where you work and takes no dir")
	}
	settings := harness.Settings{Harness: strings.TrimSpace(args.Harness), Model: strings.TrimSpace(args.Model), Effort: strings.TrimSpace(args.Effort)}
	return session.ChildOptions{Name: args.Name, Title: args.Title, Settings: settings, Dir: args.Dir, Message: args.Message, Fork: args.Fork}, nil
}

func endSession(rt *session.Runtime, self, name string) (string, error) {
	child, err := rt.EndChild(self, name)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Ended %s.", child.Name), nil
}
