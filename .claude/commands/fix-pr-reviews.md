# Fix PR review comments

For the current branch, fetch all open pull request review comments using `gh pr view --json comments`. For each comment that has not yet been resolved:

1. Read the referenced file and line to understand the context.
2. Apply the fix directly in the file.
3. Reply to the comment inline using `gh api repos/{owner}/{repo}/pulls/comments/{comment_id}/replies -f body="<reply>"`, explaining what you changed.

After all comments are addressed, stage and commit the changes, then push the branch.

Keep replies short and factual: what was changed and why. Do not ask for confirmation before fixing — just fix and reply.