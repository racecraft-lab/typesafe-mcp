package main

import (
	"strings"
	"testing"
)

// REL-01: every release path points at the fork. An updater that falls back to
// upstream would replace this binary with one that has none of its changes:
// no explicit backend selection, no credential rules, no validation.
func TestReleaseSourceIsTheFork(t *testing.T) {
	if githubRepo != "racecraft-lab/typesafe-mcp" {
		t.Fatalf("githubRepo = %q", githubRepo)
	}
	if strings.Contains(githubRepo, "itsmostafa") {
		t.Fatal("the updater points at upstream")
	}
	if !strings.Contains(attributionURL, "racecraft-lab") {
		t.Errorf("attribution URL = %q", attributionURL)
	}
}

// REL-02: an older or equal release never replaces the running build, and a
// development build refuses to self-update at all.
func TestVersionComparison(t *testing.T) {
	for _, tc := range []struct {
		latest, current string
		wantNewer       bool
	}{
		{"v0.5.0", "v0.4.0", true},
		{"v0.4.1", "v0.4.0", true},
		{"v1.0.0", "v0.99.99", true},
		// The case upstream got wrong: unequal strings meant "update", so an
		// older release replaced a newer local build.
		{"v0.4.0", "v0.5.0", false},
		{"v0.4.0", "v0.4.0", false},
		{"v0.4.0", "v0.4.1", false},
		// A pre-release leads to its version, so it sorts before it.
		{"v0.5.0", "v0.5.0-rc.1", true},
		{"v0.5.0-rc.1", "v0.5.0", false},
		{"v0.5.0-rc.2", "v0.5.0-rc.1", true},
	} {
		latest, err := parseVersion(tc.latest)
		if err != nil {
			t.Fatalf("%s: %v", tc.latest, err)
		}
		current, err := parseVersion(tc.current)
		if err != nil {
			t.Fatalf("%s: %v", tc.current, err)
		}
		if got := latest.newerThan(current); got != tc.wantNewer {
			t.Errorf("%s newerThan %s = %v, want %v", tc.latest, tc.current, got, tc.wantNewer)
		}
	}
}

// A build that is not a release has nothing to compare, so parsing must fail
// and runUpdate must refuse rather than guess.
func TestDevelopmentBuildsHaveNoVersion(t *testing.T) {
	for _, v := range []string{"dev", "", "(devel)", "v0.4.0+dirty", "main", "0.4"} {
		if _, err := parseVersion(v); err == nil {
			t.Errorf("parseVersion(%q) should fail; a non-release must not self-update", v)
		}
	}
	// Release tags in both common spellings do parse.
	for _, v := range []string{"v0.4.0", "0.4.0", "v1.2.3-rc.1"} {
		if _, err := parseVersion(v); err != nil {
			t.Errorf("parseVersion(%q): %v", v, err)
		}
	}
}
