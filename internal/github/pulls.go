package github

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"
)

// pullRequestsQuery reads the open pull requests, in every repository, that
// involve the account: ones it wrote, was asked to review, reviewed, commented
// on, or was mentioned in. requested reads the ones whose review was requested
// from the account or one of its teams, which involved can crowd out, and
// direct lists the ones requested from the account itself. It reads the first
// 20 review threads of each; remainingThreads reads the rest.
const pullRequestsQuery = threadFragment + `
fragment pr on PullRequest{
  id number title url updatedAt isDraft isCrossRepository headRefName headRefOid
  repository{nameWithOwner}
  author{login} reviewDecision mergeStateStatus statusCheckRollup{state}
  reviewRequests{totalCount}
  latestOpinionatedReviews(first:10){nodes{author{login} state commit{oid}}}
  reviews(last:1,author:$login,states:[APPROVED,CHANGES_REQUESTED,COMMENTED,DISMISSED]){nodes{submittedAt commit{oid}}}
  reviewThreads(first:20){pageInfo{hasNextPage endCursor} nodes{...thread}}
  comments(last:20){nodes{author{__typename login} createdAt}}
  commits(last:1){nodes{commit{committedDate}}}}
query($login:String!,$involved:String!,$requested:String!,$direct:String!){
  involved:search(query:$involved,type:ISSUE_ADVANCED,first:50){nodes{...pr}}
  requested:search(query:$requested,type:ISSUE,first:20){nodes{...pr}}
  direct:search(query:$direct,type:ISSUE,first:50){nodes{...on PullRequest{url}}}}`

// threadFragment reads a review thread's state and its first and last
// comments.
const threadFragment = `fragment thread on PullRequestReviewThread{isResolved
  first:comments(first:1){nodes{author{__typename login} createdAt}}
  last:comments(last:1){nodes{author{__typename login} createdAt}}}`

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

// reviewThreads is a page of a pull request's review threads.
type reviewThreads struct {
	PageInfo struct {
		HasNextPage bool   `json:"hasNextPage"`
		EndCursor   string `json:"endCursor"`
	} `json:"pageInfo"`
	Nodes []struct {
		IsResolved bool     `json:"isResolved"`
		First      comments `json:"first"`
		Last       comments `json:"last"`
	} `json:"nodes"`
}

// pullRequestNode is a pull request as pullRequestsQuery reads it.
type pullRequestNode struct {
	ID         string    `json:"id"`
	Number     int       `json:"number"`
	Title      string    `json:"title"`
	URL        string    `json:"url"`
	UpdatedAt  time.Time `json:"updatedAt"`
	Repository struct {
		NameWithOwner string `json:"nameWithOwner"`
	} `json:"repository"`
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
	ReviewThreads reviewThreads `json:"reviewThreads"`
	Comments      comments      `json:"comments"`
	Commits       struct {
		Nodes []struct {
			Commit struct {
				CommittedDate time.Time `json:"committedDate"`
			} `json:"commit"`
		} `json:"nodes"`
	} `json:"commits"`
}

// accountPullRequests lists the open pull requests, in every repository, that
// involve the account, never nil, most recently updated first with any further
// review requests after them.
func (w *Watcher) accountPullRequests(ctx context.Context, token, login string) ([]PullRequest, error) {
	var data struct {
		Involved struct {
			Nodes []pullRequestNode `json:"nodes"`
		} `json:"involved"`
		Requested struct {
			Nodes []pullRequestNode `json:"nodes"`
		} `json:"requested"`
		Direct struct {
			Nodes []struct {
				URL string `json:"url"`
			} `json:"nodes"`
		} `json:"direct"`
	}
	search := "is:pr is:open "
	// TODO: Remove once kkestell/hypercube-pr-lab is no longer needed. It
	// holds test pull requests, so only the dev session lists them.
	if os.Getenv("HYPERCUBE_DEV_UI") == "" {
		search += "-repo:kkestell/hypercube-pr-lab "
	}
	variables := map[string]any{"login": login,
		"involved":  search + "sort:updated-desc (involves:@me OR review-requested:@me OR reviewed-by:@me)",
		"requested": search + "review-requested:@me",
		"direct":    search + "user-review-requested:@me"}
	if err := w.query(ctx, token, pullRequestsQuery, variables, &data); err != nil {
		return nil, err
	}
	requested, direct, seen := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, n := range data.Requested.Nodes {
		requested[n.URL] = true
	}
	for _, n := range data.Direct.Nodes {
		direct[n.URL] = true
	}
	var nodes []pullRequestNode
	for _, n := range append(data.Involved.Nodes, data.Requested.Nodes...) {
		if n.URL == "" || seen[n.URL] { // not a pull request, or listed already
			continue
		}
		seen[n.URL] = true
		nodes = append(nodes, n)
	}
	if err := w.remainingThreads(ctx, token, nodes); err != nil {
		return nil, err
	}
	prs := []PullRequest{}
	var failing []PullRequest // the account's pull requests with failing checks
	for _, n := range nodes {
		pr := fromNode(n, login, requested[n.URL], direct[n.URL])
		if pr.Author == login && (pr.Checks == "FAILURE" || pr.Checks == "ERROR") {
			failing = append(failing, pr)
		}
		prs = append(prs, pr)
	}
	if len(failing) > 0 {
		required, err := w.requiredFailures(ctx, token, failing)
		if err != nil {
			return nil, err
		}
		for i := range prs {
			prs[i].RequiredFailed = required[prs[i].URL]
		}
	}
	return prs, nil
}

// remainingThreads reads the review threads after the first page of each
// pull request that has more, adding them to its node. Each round reads the
// next page of every such pull request in one query, with a field per pull
// request.
func (w *Watcher) remainingThreads(ctx context.Context, token string, nodes []pullRequestNode) error {
	for {
		var pending []int
		for i := range nodes {
			if nodes[i].ReviewThreads.PageInfo.HasNextPage {
				pending = append(pending, i)
			}
		}
		if len(pending) == 0 {
			return nil
		}
		var params, fields strings.Builder
		variables := map[string]any{}
		for k, i := range pending {
			fmt.Fprintf(&params, "$i%d:ID!,$c%d:String!,", k, k)
			fmt.Fprintf(&fields, "p%d:node(id:$i%d){...on PullRequest{reviewThreads(first:100,after:$c%d){pageInfo{hasNextPage endCursor} nodes{...thread}}}}\n", k, k, k)
			variables[fmt.Sprintf("i%d", k)] = nodes[i].ID
			variables[fmt.Sprintf("c%d", k)] = nodes[i].ReviewThreads.PageInfo.EndCursor
		}
		query := threadFragment + "\nquery(" + strings.TrimSuffix(params.String(), ",") + "){\n" + fields.String() + "}"
		var data map[string]*struct {
			ReviewThreads reviewThreads `json:"reviewThreads"`
		}
		if err := w.query(ctx, token, query, variables, &data); err != nil {
			return err
		}
		for k, i := range pending {
			page := data[fmt.Sprintf("p%d", k)]
			if page == nil {
				return fmt.Errorf("reading the review threads of %s: no pull request", nodes[i].URL)
			}
			threads := &nodes[i].ReviewThreads
			threads.Nodes = append(threads.Nodes, page.ReviewThreads.Nodes...)
			threads.PageInfo = page.ReviewThreads.PageInfo
		}
	}
}

// fromNode describes the pull request as the account sees it. requested says
// its review was requested from the account or one of its teams, and direct
// from the account itself.
func fromNode(n pullRequestNode, login string, requested, direct bool) PullRequest {
	pr := PullRequest{Number: n.Number, Title: n.Title, URL: n.URL, Repo: n.Repository.NameWithOwner, Branch: n.HeadRefName,
		Fork: n.IsCrossRepository, Draft: n.IsDraft, Review: n.ReviewDecision, Merge: n.MergeStateStatus,
		PendingReviews: n.ReviewRequests.TotalCount}
	if !n.UpdatedAt.IsZero() {
		pr.Updated = n.UpdatedAt.UnixMilli()
	}
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
	// After a push answers every change request, the request's own threads
	// wait for the reviewers to look again; only newer comments count.
	answered := mine && pr.Review == "CHANGES_REQUESTED" && pr.Responded
	for _, t := range n.ReviewThreads.Nodes {
		first, _ := t.First.last()
		last, ok := t.Last.last()
		if !t.IsResolved && ok && last.other(login) && (mine || first.person() == login) &&
			(!answered || afterLastCommit(n, last)) {
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
			if afterLastCommit(n, c) {
				return 1
			}
			return 0
		}
	}
	return 0
}

// afterLastCommit reports whether the comment came after the pull request's
// last commit, or the pull request has no commits.
func afterLastCommit(n pullRequestNode, c comment) bool {
	return len(n.Commits.Nodes) == 0 || c.CreatedAt.After(n.Commits.Nodes[0].Commit.CommittedDate)
}

// requiredFailures reports, by URL, which of the pull requests have a failed
// required check. GitHub says whether a check is required only for one pull
// request at a time, so each pull request gets its own field in the query,
// grouped by repository. Owner and name come from GitHub's nameWithOwner, so
// quoting them is safe.
func (w *Watcher) requiredFailures(ctx context.Context, token string, failing []PullRequest) (map[string]bool, error) {
	var fields strings.Builder
	type slot struct{ repo, field, url string }
	var slots []slot
	byRepo := map[string][]PullRequest{}
	var repos []string
	for _, pr := range failing {
		if _, ok := byRepo[pr.Repo]; !ok {
			repos = append(repos, pr.Repo)
		}
		byRepo[pr.Repo] = append(byRepo[pr.Repo], pr)
	}
	for i, repo := range repos {
		owner, name, _ := strings.Cut(repo, "/")
		fmt.Fprintf(&fields, "r%d:repository(owner:%q,name:%q){", i, owner, name)
		for _, pr := range byRepo[repo] {
			n := pr.Number
			fmt.Fprintf(&fields, `p%d:pullRequest(number:%d){statusCheckRollup{contexts(first:100){nodes{
  ...on CheckRun{conclusion isRequired(pullRequestNumber:%d)}
  ...on StatusContext{state isRequired(pullRequestNumber:%d)}}}}}`, n, n, n, n)
			slots = append(slots, slot{fmt.Sprintf("r%d", i), fmt.Sprintf("p%d", n), pr.URL})
		}
		fields.WriteString("}")
	}
	var data map[string]map[string]*struct {
		StatusCheckRollup *struct {
			Contexts struct {
				Nodes []struct {
					Conclusion string `json:"conclusion"`
					State      string `json:"state"`
					IsRequired bool   `json:"isRequired"`
				} `json:"nodes"`
			} `json:"contexts"`
		} `json:"statusCheckRollup"`
	}
	if err := w.query(ctx, token, "query{"+fields.String()+"}", nil, &data); err != nil {
		return nil, err
	}
	failed := map[string]bool{}
	for _, s := range slots {
		pr := data[s.repo][s.field]
		if pr == nil || pr.StatusCheckRollup == nil {
			continue
		}
		for _, c := range pr.StatusCheckRollup.Contexts.Nodes {
			bad := slices.Contains([]string{"FAILURE", "TIMED_OUT", "CANCELLED", "ACTION_REQUIRED", "STARTUP_FAILURE"}, c.Conclusion) ||
				c.State == "FAILURE" || c.State == "ERROR"
			if c.IsRequired && bad {
				failed[s.url] = true
			}
		}
	}
	return failed, nil
}
