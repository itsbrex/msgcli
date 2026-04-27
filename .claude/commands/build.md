Run the full build and test suite for msgcli.

```bash
cd /Users/hack/github/msgcli && make build && go vet ./... && go test ./...
```

Report results clearly: build status, vet warnings (if any), test results (pass/fail count).
