// The icon of a session's open pull request: its shape and color say whether
// the pull request is a draft, failing, waiting on changes, or approved.
import {Icon} from '../shared/icon.tsx';
import type {PullRequest} from './types.ts';
import {pullRequestFacts, pullRequestLook} from './wording.ts';

export function PullRequestIcon({pr, className = ''}: {pr: PullRequest; className?: string}) {
  const {icon, color} = pullRequestLook(pr);
  const title = [`Pull request #${pr.number}: ${pr.title}.`, ...pullRequestFacts(pr)].join(' ');
  return <span className={`inline-flex ${className}`} title={title}>
    <Icon name={icon} className={color} title={title} role="img" />
  </span>;
}
