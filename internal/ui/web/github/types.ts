// A GitHub workspace's status and pull requests as the server sends them.

// PullRequest is an open pull request. review is APPROVED, CHANGES_REQUESTED,
// REVIEW_REQUIRED, or empty. checks is SUCCESS, PENDING, EXPECTED, FAILURE,
// ERROR, or empty when there are none.
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
}

// GitHubStatus is a workspace's repository, account, branch tracking, and open
// pull requests. choices lists the accounts to choose among when several
// qualify. updated is when the last query succeeded, in Unix milliseconds, or
// 0 before one; error is why the last update failed.
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
