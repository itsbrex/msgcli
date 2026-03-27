package auth

import "testing"

func TestResolveAuthFlowPrecedence(t *testing.T) {
	originalEnvGetter := envGetter
	t.Cleanup(func() {
		envGetter = originalEnvGetter
	})

	cfg := &Config{DefaultAuthFlow: FlowMSALOffice}

	envGetter = func(key string) string {
		if key == EnvAuthFlow {
			return "legacy"
		}
		return ""
	}

	flow, err := ResolveAuthFlow("msal-office", cfg)
	if err != nil {
		t.Fatalf("ResolveAuthFlow returned error: %v", err)
	}
	if flow != FlowMSALOffice {
		t.Fatalf("expected flag to win, got %s", flow)
	}

	flow, err = ResolveAuthFlow("", cfg)
	if err != nil {
		t.Fatalf("ResolveAuthFlow returned error: %v", err)
	}
	if flow != FlowLegacy {
		t.Fatalf("expected env to win, got %s", flow)
	}

	envGetter = func(string) string { return "" }
	flow, err = ResolveAuthFlow("", cfg)
	if err != nil {
		t.Fatalf("ResolveAuthFlow returned error: %v", err)
	}
	if flow != FlowMSALOffice {
		t.Fatalf("expected config default to win, got %s", flow)
	}

	flow, err = ResolveAuthFlow("", nil)
	if err != nil {
		t.Fatalf("ResolveAuthFlow returned error: %v", err)
	}
	if flow != FlowLegacy {
		t.Fatalf("expected final fallback legacy, got %s", flow)
	}
}

func TestResolveAuthFlowRejectsInvalidValues(t *testing.T) {
	originalEnvGetter := envGetter
	t.Cleanup(func() {
		envGetter = originalEnvGetter
	})

	envGetter = func(key string) string {
		if key == EnvAuthFlow {
			return "invalid"
		}
		return ""
	}

	if _, err := ResolveAuthFlow("", nil); err == nil {
		t.Fatalf("expected invalid env flow to fail")
	}
	if _, err := ResolveAuthFlow("invalid", nil); err == nil {
		t.Fatalf("expected invalid flag flow to fail")
	}
}

func TestParseAuthFlowBackCompatAlias(t *testing.T) {
	flow, err := ParseAuthFlow("msal-firstparty")
	if err != nil {
		t.Fatalf("ParseAuthFlow returned error: %v", err)
	}
	if flow != FlowMSALOffice {
		t.Fatalf("expected deprecated alias to normalize to %s, got %s", FlowMSALOffice, flow)
	}
}
