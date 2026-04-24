package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/skylarbpayne/msgcli/internal/graph"
)

// confirmBulk asks the user to confirm an N-item operation, returning true if
// the user approved or the prompt was bypassed (--force / --no-input).
func confirmBulk(verb string, n int, force bool) bool {
	if force || IsNoInput() {
		return true
	}
	fmt.Fprintf(os.Stderr, "%s %d messages? [y/N]: ", verb, n)
	reader := bufio.NewReader(os.Stdin)
	resp, _ := reader.ReadString('\n')
	resp = strings.TrimSpace(strings.ToLower(resp))
	return resp == "y" || resp == "yes"
}

// reportBulkOutcome prints a summary of a batch mail operation to stderr
// (success count) and stdout (failure detail, if any). Returns nil even on
// partial failure — callers distinguish via exit code by checking any non-OK.
func reportBulkOutcome(verb string, ids []string, responses []graph.BatchResponse) error {
	ok, failed := 0, 0
	for _, r := range responses {
		if r.OK() {
			ok++
		} else {
			failed++
		}
	}
	Infof("%s %d of %d messages", verb, ok, len(ids))
	if failed > 0 {
		fmt.Fprintln(os.Stderr, "Failures:")
		for i, r := range responses {
			if !r.OK() {
				fmt.Fprintf(os.Stderr, "  %s: HTTP %d\n", ids[i], r.Status)
			}
		}
		return fmt.Errorf("%d of %d operations failed", failed, len(ids))
	}
	return nil
}
