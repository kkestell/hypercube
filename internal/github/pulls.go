package github

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"
)

// pullRequestsQuery reads the open pull request count and the open pull
// requests that involve the account: ones it wrote, was asked to review,
// reviewed, commented on, or was mentioned in. requested reads the ones whose
// review was requested from the account or one of its teams, which involved
// can crowd out, and direct numbers the ones requested from the account
// itself.
const pullRequestsQuery = `fragment pr on PullRequest{
  number title url isDraft isCrossRepository headRefName headRefOid
  author{login} reviewDecision mergeStateStatus statusCheckRollup{state}
  reviewRequests{totalCount}
  latestOpinionatedReviews(first:10){nodes{author{login} state commit{oid}}}
  reviews(last:1,author:$login,states:[APPROVED,CHANGES_REQUESTED,COMMENTED,DISMISSED]){nodes{submittedAt commit{oid}}}
  reviewThreads(first:20){nodes{isResolved
    first:comments(first:1){nodes{author{__typename login} createdAt}}
    last:comments(last:1){nodes{author{__typename login} createdAt}}}}
  comments(last:20){nodes{author{__typename login} createdAt}}
  commits(last:1){nodes{commit{committedDate}}}}
query($owner:String!,$name:String!,$login:String!,$involved:String!,$requested:String!,$direct:String!){
  repository(owner:$owner,name:$name){pullRequests(states:OPEN){totalCount}}
  involved:search(query:$involved,type:ISSUE_ADVANCED,first:20){nodes{...pr}}
  requested:search(query:$requested,type:ISSUE,first:20){nodes{...pr}}
  direct:search(query:$direct,type:ISSUE,first:20){nodes{...on PullRequest{number}}}}`

// comment is a comment or review comment and who wrote it.
type comment struct {
	Author *struct {
		Type  string `json:"__typename"`
		Login string `json:"login"`
	} `json:"author"`
	CreatedAt time.Time `json:"createdAt"`
}

// person is the login of the person who wrote the comment, or empty for a bot
// or a deleted account.
func (c comment) person() string {
	if c.Author == nil || c.Author.Type != "User" {
		return ""
	}
	return c.Author.Login
}

// other reports whether a person other than login wrote the comment.
func (c comment) other(login string) bool {
	p := c.person()
	return p != "" && p != login
}

type comments struct {
	Nodes []comment `json:"nodes"`
}

func (c comments) last() (comment, bool) {
	if len(c.Nodes) == 0 {
		return comment{}, false
	}
	return c.Nodes[len(c.Nodes)-1], true
}

// opinion is a reviewer's latest approval or change request and the commit it
// reviewed.
type opinion struct {
	Author *struct {
		Login string `json:"login"`
	} `json:"author"`
	State  string `json:"state"`
	Commit *struct {
		Oid string `json:"oid"`
	} `json:"commit"`
}

// pullRequestNode is a pull request as pullRequestsQuery reads it.
type pullRequestNode struct {
	Number            int    `json:"number"`
	Title             string `json:"title"`
	URL               string `json:"url"`
	IsDraft           bool   `json:"isDraft"`
	IsCrossRepository bool   `json:"isCrossRepository"`
	HeadRefName       string `json:"headRefName"`
	HeadRefOid        string `json:"headRefOid"`
	Author            *struct {
		Login string `json:"login"`
	} `json:"author"`
	ReviewDecision    string `json:"reviewDecision"`
	MergeStateStatus  string `json:"mergeStateStatus"`
	StatusCheckRollup *struct {
		State string `json:"state"`
	} `json:"statusCheckRollup"`
	ReviewRequests struct {
		TotalCount int `json:"totalCount"`
	} `json:"reviewRequests"`
	LatestOpinionatedReviews struct {
		Nodes []opinion `json:"nodes"`
	} `json:"latestOpinionatedReviews"`
	Reviews struct {
		Nodes []struct {
			SubmittedAt time.Time `json:"submittedAt"`
			Commit      *struct {
				Oid string `json:"oid"`
			} `json:"commit"`
		} `json:"nodes"`
	} `json:"reviews"`
	ReviewThreads struct {
		Nodes []struct {
			IsResolved bool     `json:"isResolved"`
			First      comments `json:"first"`
			Last       comments `json:"last"`
		} `json:"nodes"`
	} `json:"reviewThreads"`
	Comments comments `json:"comments"`
	Commits  struct {
		Nodes []struct {
			Commit struct {
				CommittedDate time.Time `json:"committedDate"`
			} `json:"commit"`
		} `json:"nodes"`
	} `json:"commits"`
}

// pullRequests lists the open pull requests that involve the account, never
// nil, most recently updated first with any further review requests after
// them, and counts all open pull requests.
func (w *Watcher) pullRequests(ctx context.Context, token, login, owner, name string) ([]PullRequest, int, error) {
	var data struct {
		Repository struct {
			PullRequests struct {
				TotalCount int `json:"totalCount"`
			} `json:"pullRequests"`
		} `json:"repository"`
		Involved struct {
			Nodes []pullRequestNode `json:"nodes"`
		} `json:"involved"`
		Requested struct {
			Nodes []pullRequestNode `json:"nodes"`
		} `json:"requested"`
		Direct struct {
			Nodes []struct {
				Number int `json:"number"`
			} `json:"nodes"`
		} `json:"direct"`
	}
	repo := fmt.Sprintf("repo:%s/%s is:pr is:open", owner, name)
	variables := map[string]any{"owner": owner, "name": name, "login": login,
		"involved":  repo + " sort:updated-desc (involves:@me OR review-requested:@me OR reviewed-by:@me)",
		"requested": repo + " review-requested:@me",
		"direct":    repo + " user-review-requested:@me"}
	if err := w.query(ctx, token, pullRequestsQuery, variables, &data); err != nil {
		return nil, 0, err
	}
	requested, direct, seen := map[int]bool{}, map[int]bool{}, map[int]bool{}
	for _, n := range data.Requested.Nodes {
		requested[n.Number] = true
	}
	for _, n := range data.Direct.Nodes {
		direct[n.Number] = true
	}
	prs := []PullRequest{}
	var failing []int // the account's pull requests with failing checks
	for _, n := range append(data.Involved.Nodes, data.Requested.Nodes...) {
		if n.Number == 0 || seen[n.Number] { // not a pull request, or listed already
			continue
		}
		seen[n.Number] = true
		pr := fromNode(n, login, requested[n.Number], direct[n.Number])
		if pr.Author == login && (pr.Checks == "FAILURE" || pr.Checks == "ERROR") {
			failing = append(failing, pr.Number)
		}
		prs = append(prs, pr)
	}
	if len(failing) > 0 {
		required, err := w.requiredFailures(ctx, token, owner, name, failing)
		if err != nil {
			return nil, 0, err
		}
		for i := range prs {
			prs[i].RequiredFailed = required[prs[i].Number]
		}
	}
	return prs, data.Repository.PullRequests.TotalCount, nil
}

// fromNode describes the pull request as the account sees it. requested says
// its review was requested from the account or one of its teams, and direct
// from the account itself.
func fromNode(n pullRequestNode, login string, requested, direct bool) PullRequest {
	pr := PullRequest{Number: n.Number, Title: n.Title, URL: n.URL, Branch: n.HeadRefName,
		Fork: n.IsCrossRepository, Draft: n.IsDraft, Review: n.ReviewDecision, Merge: n.MergeStateStatus,
		PendingReviews: n.ReviewRequests.TotalCount}
	if n.Author != nil {
		pr.Author = n.Author.Login
	}
	if n.StatusCheckRollup != nil {
		pr.Checks = n.StatusCheckRollup.State
	}
	switch {
	case direct:
		pr.Requested = "user"
	case requested:
		pr.Requested = "team"
	}
	for _, r := range n.LatestOpinionatedReviews.Nodes {
		if r.Author != nil && r.Author.Login == login {
			pr.Reviewed = r.State
		}
	}
	if len(n.Reviews.Nodes) > 0 {
		last := n.Reviews.Nodes[0]
		after := func(c comment) bool {
			return pr.Author != "" && c.person() == pr.Author && c.CreatedAt.After(last.SubmittedAt)
		}
		pr.Responded = last.Commit == nil || last.Commit.Oid != n.HeadRefOid || slices.ContainsFunc(n.Comments.Nodes, after)
		for _, t := range n.ReviewThreads.Nodes {
			if c, ok := t.Last.last(); ok && after(c) {
				pr.Responded = true
			}
		}
	}
	mine := pr.Author == login
	if mine && pr.Review == "CHANGES_REQUESTED" {
		// A push after every change request answers them until the
		// reviewers look again.
		pr.Responded = !slices.ContainsFunc(n.LatestOpinionatedReviews.Nodes, func(r opinion) bool {
			return r.State == "CHANGES_REQUESTED" && r.Commit != nil && r.Commit.Oid == n.HeadRefOid
		})
	}
	for _, t := range n.ReviewThreads.Nodes {
		first, _ := t.First.last()
		last, ok := t.Last.last()
		if !t.IsResolved && ok && last.other(login) && (mine || first.person() == login) {
			pr.Unanswered++
		}
	}
	if mine {
		pr.Unanswered += unansweredConversation(n, login)
	}
	return pr
}

// unansweredConversation is 1 when the last comment a person left on the pull
// request's conversation is someone else's and came after its last commit, so
// neither a reply nor a push has answered it, and 0 otherwise. GitHub gives a
// commit's time, not its push's, so a commit made before the comment and
// pushed after it does not answer it.
func unansweredConversation(n pullRequestNode, login string) int {
	for i := len(n.Comments.Nodes) - 1; i >= 0; i-- {
		c := n.Comments.Nodes[i]
		if c.person() == login {
			return 0
		}
		if c.other(login) {
			if len(n.Commits.Nodes) > 0 && !c.CreatedAt.After(n.Commits.Nodes[0].Commit.CommittedDate) {
				return 0
			}
			return 1
		}
	}
	return 0
}

// requiredFailures reports which of the pull requests have a failed required
// check. GitHub says whether a check is required only for one pull request at
// a time, so each pull request gets its own field in the query.
func (w *Watcher) requiredFailures(ctx context.Context, token, owner, name string, numbers []int) (map[int]bool, error) {
	var fields strings.Builder
	for _, n := range numbers {
		fmt.Fprintf(&fields, `p%d:pullRequest(number:%d){statusCheckRollup{contexts(first:100){nodes{
  ...on CheckRun{conclusion isRequired(pullRequestNumber:%d)}
  ...on StatusContext{state isRequired(pullRequestNumber:%d)}}}}}`, n, n, n, n)
	}
	query := `query($owner:String!,$name:String!){repository(owner:$owner,name:$name){` + fields.String() + `}}`
	var data struct {
		Repository map[string]*struct {
			StatusCheckRollup *struct {
				Contexts struct {
					Nodes []struct {
						Conclusion string `json:"conclusion"`
						State      string `json:"state"`
						IsRequired bool   `json:"isRequired"`
					} `json:"nodes"`
				} `json:"contexts"`
			} `json:"statusCheckRollup"`
		} `json:"repository"`
	}
	if err := w.query(ctx, token, query, map[string]any{"owner": owner, "name": name}, &data); err != nil {
		return nil, err
	}
	failed := map[int]bool{}
	for _, n := range numbers {
		pr := data.Repository[fmt.Sprintf("p%d", n)]
		if pr == nil || pr.StatusCheckRollup == nil {
			continue
		}
		for _, c := range pr.StatusCheckRollup.Contexts.Nodes {
			bad := slices.Contains([]string{"FAILURE", "TIMED_OUT", "CANCELLED", "ACTION_REQUIRED", "STARTUP_FAILURE"}, c.Conclusion) ||
				c.State == "FAILURE" || c.State == "ERROR"
			if c.IsRequired && bad {
				failed[n] = true
			}
		}
	}
	return failed, nil
}
