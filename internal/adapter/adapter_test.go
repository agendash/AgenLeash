package adapter

import (
	"path/filepath"
	"runtime"
	"testing"
)

func TestVersionRangeMatches(t *testing.T) {
	rng, err := ParseVersionRange(">=1.10.0 <2.0.0")
	if err != nil {
		t.Fatalf("ParseVersionRange: %v", err)
	}

	cases := []struct {
		version string
		want    bool
	}{
		{version: "1.9.9", want: false},
		{version: "1.10.0", want: true},
		{version: "1.12.3", want: true},
		{version: "2.0.0", want: false},
	}

	for _, tc := range cases {
		got, err := rng.Matches(tc.version)
		if err != nil {
			t.Fatalf("Matches(%q): %v", tc.version, err)
		}
		if got != tc.want {
			t.Fatalf("Matches(%q) = %v, want %v", tc.version, got, tc.want)
		}
	}
}

func TestLoadDirectoryAndResolve(t *testing.T) {
	root := repoRoot(t)

	specs, err := LoadDirectory(filepath.Join(root, "adapters"))
	if err != nil {
		t.Fatalf("LoadDirectory: %v", err)
	}
	if len(specs) < 4 {
		t.Fatalf("LoadDirectory loaded %d specs, want at least 4", len(specs))
	}

	var (
		claudecode AdapterSpec
		codex      AdapterSpec
		opencode   AdapterSpec
	)
	loadedNames := map[string]bool{}
	for _, spec := range specs {
		loadedNames[spec.Metadata.Name] = true
		if spec.Metadata.Name == "claudecode" {
			claudecode = spec
		}
		if spec.Metadata.Name == "codex" {
			codex = spec
		}
		if spec.Metadata.Name == "opencode" {
			opencode = spec
		}
	}
	if claudecode.Metadata.Name == "" {
		t.Fatalf("claudecode spec not loaded")
	}
	if codex.Metadata.Name == "" {
		t.Fatalf("codex spec not loaded")
	}
	if opencode.Metadata.Name == "" {
		t.Fatalf("opencode spec not loaded")
	}
	for _, name := range []string{
		"codex_local",
		"claude_local",
		"opencode_local",
		"cursor",
		"gemini_local",
		"grok_local",
		"pi_local",
		"acpx_local",
	} {
		if !loadedNames[name] {
			t.Fatalf("%s spec not loaded", name)
		}
	}

	effective, err := Resolve(claudecode, "1.12.0")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if effective.Profile != "stable" {
		t.Fatalf("Profile = %q, want stable", effective.Profile)
	}
	if !effective.Capabilities.SupportsResume {
		t.Fatalf("SupportsResume = false, want true")
	}
	if !effective.Capabilities.SupportsNativeConversation {
		t.Fatalf("SupportsNativeConversation = false, want true")
	}
	if !effective.Features["toolCallEvents"] {
		t.Fatalf("toolCallEvents = false, want true")
	}
	if !effective.Features["structuredPatch"] {
		t.Fatalf("structuredPatch = false, want true")
	}
	if !effective.Features["x.claudecode.reviewLoop"] {
		t.Fatalf("custom feature not preserved")
	}

	legacy, err := Resolve(claudecode, "1.8.0")
	if err != nil {
		t.Fatalf("Resolve legacy: %v", err)
	}
	if legacy.Profile != "legacy" {
		t.Fatalf("legacy Profile = %q, want legacy", legacy.Profile)
	}
	if legacy.Capabilities.SupportsResume {
		t.Fatalf("legacy SupportsResume = true, want false")
	}
	if legacy.Features["toolCallEvents"] {
		t.Fatalf("legacy toolCallEvents = true, want false")
	}

	v2, err := Resolve(claudecode, "2.1.100")
	if err != nil {
		t.Fatalf("Resolve v2: %v", err)
	}
	if v2.Profile != "v2" {
		t.Fatalf("v2 Profile = %q, want v2", v2.Profile)
	}
	if !v2.Capabilities.SupportsResume {
		t.Fatalf("v2 SupportsResume = false, want true")
	}
	if !v2.Capabilities.SupportsNativeConversation {
		t.Fatalf("v2 SupportsNativeConversation = false, want true")
	}
	if !v2.Features["planMode"] {
		t.Fatalf("v2 planMode = false, want true")
	}
}

func TestLoadFileValidatesMockAdapter(t *testing.T) {
	root := repoRoot(t)
	spec, err := LoadFile(filepath.Join(root, "adapters", "mock-adapter.json"))
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if spec.Metadata.Name != "mock-adapter" {
		t.Fatalf("Metadata.Name = %q, want mock-adapter", spec.Metadata.Name)
	}
	if spec.Spec.AgentFamily != "mockagent" {
		t.Fatalf("AgentFamily = %q, want mockagent", spec.Spec.AgentFamily)
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}
