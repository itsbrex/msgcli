Find and report technical debt in the msgcli codebase.

Scan the codebase for:

1. **Duplicated logic** — similar patterns repeated across files
   ```bash
   rg -n "TODO|FIXME|HACK|XXX" --type go
   ```

2. **Deprecated patterns** — `ioutil`, named returns, `os.Exit` outside main
   ```bash
   rg -n "ioutil\." --type go
   rg -n "os\.Exit" --type go internal/ 
   ```

3. **Missing error wrapping** — bare `return err` without context
   ```bash
   rg -n "return err$" --type go
   ```

4. **Dead code candidates** — exported symbols with no callers
   ```bash
   rg -rn "^func [A-Z]" --type go internal/
   ```

5. **Test coverage gaps** — Go files without corresponding `_test.go`
   ```bash
   find internal/ -name "*.go" ! -name "*_test.go" | sort
   find internal/ -name "*_test.go" | sort
   ```

Report: prioritized list of issues with file:line references and suggested fixes.
