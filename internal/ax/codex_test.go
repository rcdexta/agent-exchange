package ax

import (
	"path/filepath"
	"testing"
)

func TestNamedSessionPathRetainsLegacyIdentity(t *testing.T) {
	dir := testDir(t)
	legacy := filepath.Join(dir, testMesh+"_api.json")
	if err := saveSession(legacy, Session{Name: "api"}); err != nil {
		t.Fatal(err)
	}
	path, err := namedSessionPath(dir, "api")
	if err != nil || path != legacy {
		t.Fatalf("legacy identity: %s %v", path, err)
	}
	// An underscore in a new global name must not look like a legacy prefix.
	if err := saveSession(filepath.Join(dir, "other_web.json"), Session{Name: "other_web"}); err != nil {
		t.Fatal(err)
	}
	path, err = namedSessionPath(dir, "web")
	if err != nil || path != filepath.Join(dir, "web.json") {
		t.Fatalf("global name: %s %v", path, err)
	}
	if err := saveSession(filepath.Join(dir, "api.json"), Session{Name: "api"}); err != nil {
		t.Fatal(err)
	}
	if _, err = namedSessionPath(dir, "api"); err == nil {
		t.Fatal("ambiguous legacy name accepted")
	}
}

func TestCodexNativeOptionBoundary(t *testing.T) {
	if codexHelp([]string{"resume", "session", "--", "--help"}) {
		t.Fatal("literal prompt disabled automatic startup")
	}
	if !codexHelp([]string{"resume", "--help"}) {
		t.Fatal("native help started a backend")
	}
	args := []string{"-c", "model=\"a\"", "resume", "session", "--config=model_reasoning_effort=\"low\"", "--", "-cignored"}
	configs := codexConfigOverrides(args)
	if len(configs) != 2 || configs[0] != "model=\"a\"" || configs[1] != "model_reasoning_effort=\"low\"" {
		t.Fatalf("native configuration changed: %q", configs)
	}
}
