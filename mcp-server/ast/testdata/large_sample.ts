// Large synthetic TypeScript file for TestTokenReduction in ast_test.go.
// Contains multiple interfaces, type aliases, and function signatures
// alongside verbose function bodies to ensure a meaningful token-reduction ratio.

export interface Repository {
  id: number;
  name: string;
  owner: string;
  description: string;
  isPrivate: boolean;
  createdAt: Date;
  updatedAt: Date;
}

export interface Commit {
  sha: string;
  message: string;
  author: string;
  timestamp: Date;
  parentShas: string[];
}

export interface PullRequest {
  id: number;
  title: string;
  body: string;
  sourceBranch: string;
  targetBranch: string;
  status: PullRequestStatus;
  commits: Commit[];
}

export interface ReviewComment {
  id: number;
  pullRequestId: number;
  author: string;
  body: string;
  filePath: string;
  lineNumber: number;
  createdAt: Date;
  updatedAt: Date;
  resolved: boolean;
}

export interface Label {
  id: number;
  name: string;
  color: string;
  description: string;
}

export type RepositoryId = number;
export type CommitSha = string;
export type PullRequestStatus = "open" | "closed" | "merged" | "draft";
export type AuthorLogin = string;
export type LabelId = number;
export type ReviewCommentId = number;

export function createRepository(
  name: string,
  owner: string,
  isPrivate: boolean,
): Repository {
  if (!name || name.trim().length === 0) {
    throw new Error("Repository name must not be empty or whitespace only");
  }
  if (!owner || owner.trim().length === 0) {
    throw new Error("Repository owner must not be empty or whitespace only");
  }
  const now = new Date();
  const id = Math.floor(Math.random() * 1_000_000);
  console.log(`Creating repository '${name}' owned by '${owner}' (id=${id})`);
  return {
    id,
    name: name.trim(),
    owner: owner.trim(),
    description: "",
    isPrivate,
    createdAt: now,
    updatedAt: now,
  };
}

export function findCommitBySha(
  sha: CommitSha,
  commits: Commit[],
): Commit | undefined {
  if (!sha || sha.trim().length === 0) {
    throw new Error("SHA must not be empty when searching for a commit in the list");
  }
  const normalised = sha.trim().toLowerCase();
  for (const commit of commits) {
    if (commit.sha.toLowerCase() === normalised) {
      return commit;
    }
  }
  return undefined;
}

export function openPullRequest(
  title: string,
  body: string,
  sourceBranch: string,
  targetBranch: string,
): PullRequest {
  if (!title || title.trim().length === 0) {
    throw new Error("Pull request title must not be empty or whitespace only");
  }
  if (!sourceBranch || sourceBranch.trim().length === 0) {
    throw new Error("Source branch must not be empty");
  }
  if (!targetBranch || targetBranch.trim().length === 0) {
    throw new Error("Target branch must not be empty");
  }
  if (sourceBranch.trim() === targetBranch.trim()) {
    throw new Error(
      `Source branch "${sourceBranch}" and target branch "${targetBranch}" must be different`,
    );
  }
  const id = Math.floor(Math.random() * 1_000_000);
  console.log(`Opening PR #${id}: "${title}" (${sourceBranch} → ${targetBranch})`);
  return {
    id,
    title: title.trim(),
    body,
    sourceBranch: sourceBranch.trim(),
    targetBranch: targetBranch.trim(),
    status: "open",
    commits: [],
  };
}

export function mergePullRequest(pr: PullRequest): PullRequest {
  if (pr.status !== "open") {
    throw new Error(
      `Cannot merge pull request #${pr.id} ("${pr.title}") in status "${pr.status}"; ` +
        `only pull requests with status "open" can be merged`,
    );
  }
  if (pr.commits.length === 0) {
    throw new Error(
      `Cannot merge pull request #${pr.id} ("${pr.title}") because it has no commits; ` +
        `add at least one commit before attempting to merge`,
    );
  }
  console.log(
    `Merging PR #${pr.id} "${pr.title}" with ${pr.commits.length} commit(s)`,
  );
  return { ...pr, status: "merged" };
}

export function closePullRequest(pr: PullRequest): PullRequest {
  if (pr.status === "merged") {
    throw new Error(
      `Cannot close pull request #${pr.id} ("${pr.title}") because it has already been merged; ` +
        `use reopenPullRequest first if you intend to close it instead`,
    );
  }
  if (pr.status === "closed") {
    throw new Error(
      `Pull request #${pr.id} ("${pr.title}") is already in status "closed"`,
    );
  }
  console.log(`Closing PR #${pr.id} "${pr.title}"`);
  return { ...pr, status: "closed" };
}

export function listOpenPullRequests(prs: PullRequest[]): PullRequest[] {
  const open = prs.filter((pr) => pr.status === "open");
  console.log(`Found ${open.length} open pull request(s) out of ${prs.length} total`);
  return open;
}

export function addCommitToPullRequest(
  pr: PullRequest,
  commit: Commit,
): PullRequest {
  if (pr.status !== "open") {
    throw new Error(
      `Cannot add commit ${commit.sha} to pull request #${pr.id} ` +
        `because the PR is in status "${pr.status}" and only open PRs accept new commits`,
    );
  }
  const already = pr.commits.some((c) => c.sha === commit.sha);
  if (already) {
    console.warn(
      `Commit ${commit.sha} is already present in PR #${pr.id}; skipping duplicate`,
    );
    return pr;
  }
  console.log(`Adding commit ${commit.sha} to PR #${pr.id} "${pr.title}"`);
  return { ...pr, commits: [...pr.commits, commit] };
}

export function summarisePullRequest(pr: PullRequest): string {
  const commitCount = pr.commits.length;
  const plural = commitCount === 1 ? "commit" : "commits";
  const authorSet = new Set(pr.commits.map((c) => c.author));
  const authorList = Array.from(authorSet).join(", ");
  const contributors =
    authorSet.size > 0
      ? `contributors: ${authorList}`
      : "no contributors recorded";
  return (
    `PR #${pr.id}: "${pr.title}" — ` +
    `${pr.status} — ` +
    `${pr.sourceBranch} → ${pr.targetBranch} — ` +
    `${commitCount} ${plural} — ` +
    contributors
  );
}

export function resolveReviewComment(comment: ReviewComment): ReviewComment {
  if (comment.resolved) {
    console.warn(
      `Review comment #${comment.id} on PR #${comment.pullRequestId} ` +
        `in file "${comment.filePath}" at line ${comment.lineNumber} is already resolved`,
    );
    return comment;
  }
  const resolvedAt = new Date();
  console.log(
    `Resolving review comment #${comment.id} on PR #${comment.pullRequestId} ` +
      `in file "${comment.filePath}" at line ${comment.lineNumber} at ${resolvedAt.toISOString()}`,
  );
  return { ...comment, resolved: true, updatedAt: resolvedAt };
}

export function buildRepositoryUrl(repo: Repository): string {
  const visibility = repo.isPrivate ? "private" : "public";
  const created = repo.createdAt.toISOString().split("T")[0];
  const updated = repo.updatedAt.toISOString().split("T")[0];
  console.log(
    `Building URL for ${visibility} repository "${repo.name}" owned by "${repo.owner}" ` +
      `(created ${created}, last updated ${updated})`,
  );
  return `https://github.com/${repo.owner}/${repo.name}`;
}

export function filterCommitsByAuthor(
  commits: Commit[],
  author: AuthorLogin,
): Commit[] {
  if (!author || author.trim().length === 0) {
    throw new Error(
      "Author login must not be empty when filtering commits by author",
    );
  }
  const normalised = author.trim().toLowerCase();
  const filtered = commits.filter(
    (c) => c.author.trim().toLowerCase() === normalised,
  );
  console.log(
    `Filtered ${filtered.length} commit(s) out of ${commits.length} by author "${author}"`,
  );
  return filtered;
}

export function validateLabel(label: Label): void {
  if (!label.name || label.name.trim().length === 0) {
    throw new Error("Label name must not be empty");
  }
  if (!label.color || !/^#[0-9a-fA-F]{6}$/.test(label.color)) {
    throw new Error(
      `Label color "${label.color}" is invalid; expected a hex colour in the format #RRGGBB`,
    );
  }
  if (label.description && label.description.length > 200) {
    throw new Error(
      `Label description is too long (${label.description.length} chars); maximum is 200 characters`,
    );
  }
  console.log(
    `Validated label "${label.name}" with color ${label.color} and description "${label.description ?? "(none)"}"`,
  );
}
