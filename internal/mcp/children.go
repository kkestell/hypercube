package mcp

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/session"
	"github.com/kkestell/hypercube/internal/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// The ways create_session begins a child that has no accepted offer yet.
const (
	initiationOffer = "offer"
	initiationOwn   = "own_initiative"
)

type createSessionArgs struct {
	Title      string `json:"title"`
	Initiation string `json:"initiation,omitempty"`
	OfferID    string `json:"offer_id,omitempty"`
	Name       string `json:"name,omitempty"`
	Profile    string `json:"profile,omitempty"`
	Dir        string `json:"dir,omitempty"`
	Message    string `json:"message,omitempty"`
	Fork       bool   `json:"fork,omitempty"`
}

type endSessionArgs struct {
	Name string `json:"name"`
}

const createDescription = `Start or suggest a child session, in one of three forms.
- Offer: when the user asks for a separate new task, pass initiation "offer" and a short, meaningful title only. The user chooses New session or Do it here. End your turn: the answer arrives as a message. Continuing an existing task, including corrections and small follow-ups, stays in this session without an offer.
- Accepted offer: when the answer is New session, pass its offer_id with the same title, a name, a profile, and a message that briefs the child. It starts with fresh context.
- Suggestion: pass initiation "own_initiative" with the child's settings to save a card the user can start or dismiss. Use it for work you suggest yourself, and when the user asks you to start another session. No child starts until the user clicks Start.
The name is the child's address. The profile sets the child's harness, model, and effort, and the message is queued for the child before it starts. Fork copies your native conversation, requires your harness, and works where you work: in your folder when you work in the original checkout, otherwise in a new worktree from your HEAD; a fork takes no dir.`

const dirDescription = " Without dir, a fresh child gets its own branch and worktree from HEAD. With dir, an existing directory given as an absolute path, a path starting with ~/, or a path relative to your directory, the child works directly in it and Hypercube creates no branch or worktree."

const noGitDirDescription = " A fresh child needs dir: an existing directory given as an absolute path, a path starting with ~/, or a path relative to your directory, where the child works directly."

// addChildTools adds the tools a top-level session uses to manage its
// children.
func addChildTools(server *mcp.Server, rt *session.Runtime, self string, w store.Workspace) {
	cfg, cfgErr := harness.LoadConfig()
	schema, err := jsonschema.For[createSessionArgs](nil)
	if err != nil {
		panic(err)
	}
	schema.Properties["initiation"].Enum = []any{initiationOffer, initiationOwn}
	for _, p := range cfg.Profiles {
		schema.Properties["profile"].Enum = append(schema.Properties["profile"].Enum, p.Name)
	}
	description := createDescription + dirDescription
	if w.NoGit {
		description = createDescription + noGitDirDescription
	}
	mcp.AddTool(server, &mcp.Tool{Name: "create_session", Description: description, InputSchema: schema}, result(func(_ context.Context, a createSessionArgs) (string, error) {
		if cfgErr != nil {
			return "", cfgErr
		}
		return createSession(rt, self, cfg, a)
	}))
	mcp.AddTool(server, &mcp.Tool{
		Name:        "end_session",
		Description: "End a child session: its agent stops for good, and a child with a worktree also has its worktree and branch removed; Git refuses dirty worktrees and unmerged branches. The ended child stays listed for the user and cannot be messaged.",
	}, result(func(_ context.Context, a endSessionArgs) (string, error) { return endSession(rt, self, a.Name) }))
}

func createSession(rt *session.Runtime, self string, cfg harness.Config, args createSessionArgs) (string, error) {
	switch {
	case args.Initiation == initiationOffer:
		if args.OfferID != "" || args.Name != "" || args.Profile != "" || args.Dir != "" || args.Message != "" || args.Fork {
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
		options, err := childOptions(cfg, args)
		if err != nil {
			return "", err
		}
		child, err := rt.StartOffer(self, args.OfferID, options)
		if err != nil {
			return "", err
		}
		if child.Branch == "" {
			return fmt.Sprintf("Created %s in %s.", child.Name, child.Dir), nil
		}
		return fmt.Sprintf("Created %s on branch %s in %s.", child.Name, child.Branch, child.Dir), nil
	case args.Initiation == initiationOwn:
		if args.Fork && args.Dir != "" {
			return "", errors.New("a fork works where you work and takes no dir")
		}
		options, err := childOptions(cfg, args)
		if err != nil {
			return "", err
		}
		if err := rt.ProposeChild(self, options); err != nil {
			return "", err
		}
		return "Proposed " + args.Name + ". The user can start it from the card; no child has started.", nil
	default:
		return "", errors.New(`pass initiation "offer" or "own_initiative", or the offer_id of an accepted offer`)
	}
}

func childOptions(cfg harness.Config, args createSessionArgs) (session.ChildOptions, error) {
	profile, err := cfg.Profile(args.Profile)
	if err != nil {
		return session.ChildOptions{}, err
	}
	return session.ChildOptions{Name: args.Name, Title: args.Title, Profile: profile, Dir: args.Dir, Message: args.Message, Fork: args.Fork}, nil
}

func endSession(rt *session.Runtime, self, name string) (string, error) {
	child, err := rt.EndChild(self, name)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Ended %s.", child.Name), nil
}
