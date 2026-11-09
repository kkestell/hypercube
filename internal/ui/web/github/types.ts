// A GitHub workspace's status and pull requests as the server sends them.

// PullRequest is an open pull request as the workspace's account sees it.
// review is APPROVED, CHANGES_REQUESTED, REVIEW_REQUIRED, or empty. checks is
// SUCCESS, PENDING, EXPECTED, FAILURE, ERROR, or empty when there are none.
// merge is GitHub's merge state, such as CLEAN, BEHIND, DIRTY, or BLOCKED.
// requiredFailed is known only for the account's own pull requests. requested
// says the account ('user') or its team ('team') was asked to review.
// reviewed is the account's standing review. responded says the author pushed
// or commented since the account's last review, or on the account's own pull
// request with changes requested, that it pushed since. unanswered counts
// conversations waiting on the account.
export interface PullRequest {
  number: number;
  title: string;
  url: string;
  author: string;
  branch: string;
  fork: boolean;
  draft: boolean;
  review: string;
  checks: string;
  merge: string;
  requiredFailed: boolean;
  requested: '' | 'user' | 'team';
  reviewed: string;
  responded: boolean;
  unanswered: number;
}

// GitHubStatus is a workspace's repository, account, branch tracking, and the
// open pull requests that involve the account. open counts all open pull
// requests. choices lists the accounts to choose among when several qualify.
// updated is when the last query succeeded, in Unix milliseconds, or 0 before
// one; error is why the last update failed.
export interface GitHubStatus {
  repo: string;
  url: string;
  account: string;
  choices?: string[];
  branch: string;
  upstream?: string;
  ahead: number;
  behind: number;
  pullRequests: PullRequest[];
  open: number;
  updated: number;
  error?: string;
}
