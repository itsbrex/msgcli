Review recently changed Go code for quality and fix issues found.

Use the `code-simplifier` agent to:
1. Identify changed files via `git diff --name-only`
2. Review each for redundancy, idiomatic Go, error handling, and CLAUDE.md compliance
3. Make targeted fixes
4. Confirm build and tests still pass

Report what was changed and why.
