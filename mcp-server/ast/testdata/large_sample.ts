// Large synthetic TypeScript file for TestTokenReduction in ast_test.go.
// Contains multiple interfaces, type aliases, and function signatures.

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

export type RepositoryId = number;
export type CommitSha = string;
export type PullRequestStatus = "open" | "closed" | "merged" | "draft";
export type AuthorLogin = string;

export function createRepository(
  name: string,
  owner: string,
  isPrivate: boolean,
): Repository {
  return {
    id: Math.random(),
    name,
    owner,
    description: "",
    isPrivate,
    createdAt: new Date(),
    updatedAt: new Date(),
  };
}

export function findCommitBySha(
  sha: CommitSha,
  commits: Commit[],
): Commit | undefined {
  return commits.find((c) => c.sha === sha);
}

export function openPullRequest(
  title: string,
  body: string,
  sourceBranch: string,
  targetBranch: string,
): PullRequest {
  return {
    id: Math.random(),
    title,
    body,
    sourceBranch,
    targetBranch,
    status: "open",
    commits: [],
  };
}

export function mergePullRequest(pr: PullRequest): PullRequest {
  return { ...pr, status: "merged" };
}

export function closePullRequest(pr: PullRequest): PullRequest {
  return { ...pr, status: "closed" };
}

export function listOpenPullRequests(prs: PullRequest[]): PullRequest[] {
  return prs.filter((pr) => pr.status === "open");
}
