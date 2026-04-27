Create a commit, push, and open a PR for current changes.

Steps:
1. Run `git status` and `git diff --stat` to see what changed
2. Run `go build ./... && go test ./...` — stop if either fails
3. Stage all changed tracked files: `git add -u`
4. Write a commit message following conventional commits format (feat/fix/docs/refactor/test/chore)
5. Commit and push to current branch
6. Create a PR with `gh pr create` — title should be concise (<70 chars), body should explain what and why

Do NOT push to main directly. Create a branch if currently on main.
Do NOT skip hooks with --no-verify.
