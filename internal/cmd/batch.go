package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/skylarbpayne/msgcli/internal/auth"
	"github.com/skylarbpayne/msgcli/internal/graph"
	"github.com/spf13/cobra"
)

var (
	batchInputFile string
)

var batchCmd = &cobra.Command{
	Use:   "batch",
	Short: "Execute a batch of Graph API requests in one call",
	Long: `Execute multiple Graph API requests as a single $batch call (up to 20 per chunk).

Reads JSONL (one BatchRequest per line) from stdin or --file.
Each request must have: id, method, url. Optional: body, headers, dependsOn.

Output is a JSON array of responses in the same order as input.`,
	RunE: runBatch,
}

func init() {
	batchCmd.Flags().StringVarP(&batchInputFile, "file", "f", "", "Read JSONL requests from a file (default: stdin)")
	rootCmd.AddCommand(batchCmd)
}

func parseJSONLRequests(r io.Reader) ([]graph.BatchRequest, error) {
	var out []graph.BatchRequest
	s := bufio.NewScanner(r)
	// Expand scanner buffer for large request bodies.
	s.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	seen := make(map[string]int)
	lineNum := 0
	for s.Scan() {
		lineNum++
		line := strings.TrimSpace(s.Text())
		if line == "" {
			continue
		}
		var req graph.BatchRequest
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			return nil, fmt.Errorf("line %d: %w", lineNum, err)
		}
		if req.ID == "" || req.Method == "" || req.URL == "" {
			return nil, fmt.Errorf("line %d: id, method, and url are required", lineNum)
		}
		if prev, dup := seen[req.ID]; dup {
			return nil, fmt.Errorf("line %d: duplicate id %q (first seen on line %d)", lineNum, req.ID, prev)
		}
		seen[req.ID] = lineNum
		out = append(out, req)
	}
	if err := s.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func runBatch(cmd *cobra.Command, args []string) error {
	var src io.Reader = os.Stdin
	if batchInputFile != "" {
		f, err := os.Open(batchInputFile)
		if err != nil {
			return err
		}
		defer f.Close()
		src = f
	}

	reqs, err := parseJSONLRequests(src)
	if err != nil {
		return err
	}
	if len(reqs) == 0 {
		return fmt.Errorf("no requests found on input")
	}

	account, err := auth.ResolveAccount(GetAccountFlag())
	if err != nil {
		return err
	}
	client := graph.NewClient(account)

	responses, err := client.Batch(context.Background(), reqs)
	if err != nil {
		return err
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(responses)
}
