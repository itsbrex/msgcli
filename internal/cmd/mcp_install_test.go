package cmd

import (
	"encoding/json"
	"testing"
)

func TestMergeClaudeCodeMCPConfig(t *testing.T) {
	existing := `{"mcpServers":{"other":{"command":"foo","args":[]}}}`
	merged, err := mergeClaudeCodeMCPConfig([]byte(existing), "/usr/local/bin/msgcli")
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	var got map[string]map[string]map[string]interface{}
	if err := json.Unmarshal(merged, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	servers := got["mcpServers"]
	if _, ok := servers["other"]; !ok {
		t.Fatalf("existing entry lost")
	}
	msg, ok := servers["msgcli"]
	if !ok {
		t.Fatalf("msgcli entry missing")
	}
	if msg["command"] != "/usr/local/bin/msgcli" {
		t.Fatalf("unexpected command: %v", msg["command"])
	}
}

func TestMergeEmptyConfigInitializesStructure(t *testing.T) {
	merged, err := mergeClaudeCodeMCPConfig([]byte(""), "/bin/msgcli")
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	var got map[string]map[string]map[string]interface{}
	if err := json.Unmarshal(merged, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := got["mcpServers"]["msgcli"]; !ok {
		t.Fatalf("msgcli entry missing")
	}
}
