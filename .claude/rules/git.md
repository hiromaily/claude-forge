# Git Practices

## Branch rules

- **Never commit directly to `main`.** All changes must go on a feature branch and be merged via PR.
  If you realise you are on `main` with uncommitted changes, stash them, create a branch, then pop:
  ```bash
  git stash
  git checkout -b feature/your-branch-name
  git stash pop
  ```
- **Before creating a branch, sync `main` first.**
  ```bash
  git checkout main
  git pull --rebase
  git checkout -b feature/your-branch-name
  ```
  Branching from a stale `main` causes the new branch to miss upstream commits, leading to conflicts and missing content when rebasing later.

## Branch naming

Use the prefix that matches the change type:

| Prefix | When to use |
|--------|-------------|
| `feature/` | New capability or behaviour |
| `fix/` | Bug fix |
| `refactor/` | Code restructuring without behaviour change |
| `chore/` | Maintenance, dependency updates, config |
| `docs/` | Documentation only |

Example: `feature/human-interaction-points`, `fix/jq-precedence-bug`

## Commit messages

This repository uses [Conventional Commits](https://www.conventionalcommits.org/):

```
<type>(<optional scope>): <short summary>
```

Recognised types: `feat`, `fix`, `refactor`, `perf`, `test`, `docs`, `chore`

Examples from this repo:
- `feat(skill): add effort-aware flow template selection`
- `fix(pre-tool-hook): correct jq precedence in skippedPhases check`
- `perf(pre-tool-hook): combine dual jq calls into single invocations`
- `chore: forge artifacts for refactor-scripts-maintainability`

## Resolving merge conflicts

- **Check `git status` before any commit or branch operation.** Unmerged paths block rebases and cause misleading errors.
- **Never commit to `main` to resolve a conflict mid-pull.** If `git pull` fails with conflicts, resolve them, then complete the merge with `git commit` — do not create new commits on top before the merge is finished.
- **Keep the more complete/advanced version when resolving state.json conflicts.** Pipeline state files (`state.json`) record progression; always keep the version with more completed phases.

## Staging discipline

- **Stage only files relevant to the current branch's purpose.** If unrelated files are staged (e.g., state.json updates mixed with a README change), unstage them before committing:
  ```bash
  git restore --staged <unrelated-file>
  ```
- **Verify `git diff --stat HEAD` before every commit** to confirm only the intended files are included.

## Rebase hygiene

- **Rebase feature branches onto `main` after `main` is updated**, especially if `main` received upstream commits after the branch was created:
  ```bash
  git stash        # if there are unstaged changes
  git rebase main
  git stash pop
  ```
- **Never rebase a branch that has already been pushed and reviewed** without coordinating with reviewers — it rewrites history.

## General

- **Check `git status` and `git log --oneline -5` after every non-trivial operation** to confirm the repository is in the expected state.
- **Do not use `--force` push to `main` or `master`** under any circumstances.
- **Do not skip hooks** (`--no-verify`) unless explicitly instructed by the user.
