Verify, simplify, and ship the current work.

This is the full "done" workflow. Run in order:

**Step 1 — Verify**
Use the `verify-app` agent to run end-to-end verification.
If anything fails, fix it before continuing.

**Step 2 — Simplify**  
Use the `code-simplifier` agent to review changed files and fix quality issues.
Re-run `go build ./... && go test ./...` after simplification.

**Step 3 — Ship**
Run `/commit-push-pr` to commit, push, and open a PR.

Do not skip any step. Report the outcome of each step before proceeding.
