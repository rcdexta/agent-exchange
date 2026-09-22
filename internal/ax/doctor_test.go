package ax

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDoctorClaudePrivacySemantics(t *testing.T) {
	for _, key := range []string{"DO_NOT_TRACK", "DISABLE_GROWTHBOOK", "DISABLE_TELEMETRY", "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC"} {
		for _, value := range []string{"", "0", "false", "1", "true", "TRUE"} {
			t.Run(key+"="+value, func(t *testing.T) {
				var out bytes.Buffer
				doctorClaudeEnvironment(&out, func(k string) string {
					if k == key {
						return value
					}
					return ""
				})
				want := value != ""
				if key == "DO_NOT_TRACK" || key == "DISABLE_GROWTHBOOK" {
					want = value == "1" || strings.EqualFold(value, "true")
				}
				if strings.Contains(out.String(), key) != want {
					t.Fatalf("warning=%q, want warning=%t", out.String(), want)
				}
			})
		}
	}
}

func TestDoctorClaudeAuthUsesOnlySafeFields(t *testing.T) {
	for _, tc := range []struct{ name, data, want string }{
		{"privacy in settings", `{"loggedIn":true,"authMethod":"claude.ai","apiProvider":"firstParty","analyticsDisabled":true,"email":"PRIVATE_EMAIL","orgId":"PRIVATE_ORG","token":"PRIVATE_TOKEN"}`, "effective analyticsDisabled=true"},
		{"enabled", `{"loggedIn":true,"authMethod":"claude.ai","apiProvider":"firstParty","analyticsDisabled":false}`, "this alone does not prove Channels"},
		{"logged out", `{"loggedIn":false}`, "not logged in"},
		{"provider", `{"loggedIn":true,"authMethod":"api_key","apiProvider":"bedrock"}`, "unavailable through Bedrock"},
		{"console", `{"loggedIn":true,"authMethod":"api_key","apiProvider":"firstParty"}`, "first-party Anthropic authentication"},
		{"older", `{"loggedIn":true}`, "eligibility unknown"},
		{"missing analytics", `{"loggedIn":true,"authMethod":"claude.ai","apiProvider":"firstParty"}`, "does not report analyticsDisabled"},
		{"unknown values", `{"loggedIn":true,"authMethod":"PRIVATE_METHOD","apiProvider":"PRIVATE_PROVIDER"}`, "eligibility unknown"},
		{"unsupported", "PRIVATE_ERROR: unsupported command", "Claude auth: unknown"},
		{"empty", "", "Claude auth: unknown"},
		{"empty object", "{}", "Claude auth: unknown"},
		{"wrong type", `{"loggedIn":"PRIVATE_VALUE"}`, "Claude auth: unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			doctorClaudeAuth(&out, []byte(tc.data))
			if !strings.Contains(out.String(), tc.want) || strings.Contains(out.String(), "PRIVATE_") {
				t.Fatalf("unexpected diagnostic: %s", &out)
			}
		})
	}
}

func TestDoctorProbeBoundsTimeAndOutput(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	if data, err := doctorProbe(ctx, "/bin/sh", "-c", "sleep 60 & wait"); err == nil || data != nil {
		t.Fatalf("timed-out probe returned %q, %v", data, err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("probe blocked for %s", elapsed)
	}
	for _, script := range []string{"printf '%20000s' x", "printf '%20000s' x >&2"} {
		if data, err := doctorProbe(context.Background(), "/bin/sh", "-c", script); err == nil || len(data) > doctorOutputLimit {
			t.Fatalf("oversized probe: bytes=%d err=%v", len(data), err)
		}
	}
	// A wrapper that exits without closing a child's inherited pipes is bounded too.
	start = time.Now()
	if _, err := doctorProbe(context.Background(), "/bin/sh", "-c", "sleep 60 & exit 0"); err == nil {
		t.Fatal("accepted a probe with inherited pipes still open")
	}
	if time.Since(start) > 2*time.Second {
		t.Fatal("wait for inherited pipes was not bounded")
	}
	data, err := doctorProbe(context.Background(), "/bin/sh", "-c", `printf '{"loggedIn":false}'; exit 1`)
	if err == nil || string(data) != `{"loggedIn":false}` {
		t.Fatalf("lost logged-out status: %q, %v", data, err)
	}
}

func TestDoctorReportsRuntimeWithoutStartingBrokerOrLeakingSecrets(t *testing.T) {
	dir, bin := testDir(t), testDir(t)
	t.Setenv("PATH", bin)
	for _, key := range []string{"DO_NOT_TRACK", "DISABLE_GROWTHBOOK", "DISABLE_TELEMETRY", "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC"} {
		t.Setenv(key, "")
	}
	t.Setenv("DO_NOT_TRACK", "true")
	script := "#!/bin/sh\nif [ \"$1\" = auth ]; then\n  printf '%s' '{\"loggedIn\":true,\"authMethod\":\"claude.ai\",\"apiProvider\":\"firstParty\",\"analyticsDisabled\":true,\"email\":\"PRIVATE_EMAIL\"}'\nelse\n  printf '2.1.278\\n'\nfi\n"
	for _, host := range []string{"claude", "codex", "grok", "opencode", "pi"} {
		if err := os.WriteFile(filepath.Join(bin, host), []byte(script), 0700); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(dir, "session.json")
	s := Session{Name: "test", Host: "claude", ID: "agt_test", Secret: "PRIVATE_SECRET", Native: uuid()}
	if err := saveSession(path, s); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AX_SESSION_FILE", path)
	before, _ := os.ReadFile(path)
	var out bytes.Buffer
	doctor(context.Background(), dir, &out)
	for _, want := range []string{dir, socketPath(dir), "Broker: unreachable", "name=\"test\"", s.Native, "DO_NOT_TRACK", "effective analyticsDisabled=true", "pi: \"2.1.278\""} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("missing %q in %s", want, &out)
		}
	}
	if strings.Contains(out.String(), "PRIVATE_") {
		t.Fatalf("leaked private data: %s", &out)
	}
	if _, err := os.Stat(socketPath(dir)); !os.IsNotExist(err) {
		t.Fatalf("doctor started a broker: %v", err)
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(before, after) || os.Getenv("DO_NOT_TRACK") != "true" {
		t.Fatal("doctor changed session or privacy state")
	}
}
