package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/skylarbpayne/msgcli/internal/auth"
	"github.com/skylarbpayne/msgcli/internal/graph"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var (
	batchInputFile string
)

var batchCmd = &cobra.Command{
	Use:   "batch",
	Short: "Execute a batch of Graph API requests in one call",
	Long: `Execute multiple Graph API requests as a single $batch call (up to 20 per chunk).

Reads JSONL (one BatchRequest per line) from stdin or --file.
For YAML input, use --file with a .yaml or .yml extension.
Each request must have: id, method, url. Optional: body, headers, dependsOn.

Output is a JSON array of responses in the same order as input.`,
	RunE: runBatch,
}

func init() {
	batchCmd.Flags().StringVarP(&batchInputFile, "file", "f", "", "Read requests from a file (.yaml/.yml for YAML, otherwise JSONL; default: stdin)")
	rootCmd.AddCommand(batchCmd)
}

// validateRequests enforces the shared rules: non-empty id/method/url and no
// duplicate IDs. It reports errors that identify the offending position via
// the supplied positionOf callback (returns a human-readable location like
// "line 3" or "requests[1]").
func validateRequests(reqs []graph.BatchRequest, positionOf func(i int) string) error {
	seen := make(map[string]string)
	for i, req := range reqs {
		if req.ID == "" || req.Method == "" || req.URL == "" {
			return fmt.Errorf("%s: id, method, and url are required", positionOf(i))
		}
		if prev, dup := seen[req.ID]; dup {
			return fmt.Errorf("%s: duplicate id %q (first seen at %s)", positionOf(i), req.ID, prev)
		}
		seen[req.ID] = positionOf(i)
	}
	return nil
}

func parseJSONLRequests(r io.Reader) ([]graph.BatchRequest, error) {
	var out []graph.BatchRequest
	var lines []int // line numbers parallel to out, for error positioning
	s := bufio.NewScanner(r)
	// Expand scanner buffer for large request bodies.
	s.Buffer(make([]byte, 0, 64*1024), 1024*1024)
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
		out = append(out, req)
		lines = append(lines, lineNum)
	}
	if err := s.Err(); err != nil {
		return nil, err
	}
	if err := validateRequests(out, func(i int) string { return fmt.Sprintf("line %d", lines[i]) }); err != nil {
		return nil, err
	}
	return out, nil
}

func parseYAMLRequests(r io.Reader) ([]graph.BatchRequest, error) {
	var doc struct {
		Requests []graph.BatchRequest `yaml:"requests"`
	}
	dec := yaml.NewDecoder(r)
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("yaml parse: %w", err)
	}
	if err := validateRequests(doc.Requests, func(i int) string { return fmt.Sprintf("requests[%d]", i) }); err != nil {
		return nil, err
	}
	return doc.Requests, nil
}

func parseRequestsFromFile(path string) ([]graph.BatchRequest, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	switch strings.ToLower(filepath.Ext(path)) {
	case ".yaml", ".yml":
		return parseYAMLRequests(f)
	default:
		return parseJSONLRequests(f)
	}
}

func runBatch(cmd *cobra.Command, args []string) error {
	var reqs []graph.BatchRequest
	var err error
	if batchInputFile != "" {
		reqs, err = parseRequestsFromFile(batchInputFile)
	} else {
		reqs, err = parseJSONLRequests(os.Stdin)
	}
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
