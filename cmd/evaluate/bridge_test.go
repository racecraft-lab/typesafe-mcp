package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
)

// BRIDGE-01: this release line is the hop from racecraft-lab/typesafe-mcp to
// racecraft-lab/racecraft-plugins-public. It says where it was built, and
// where its updater now reads.
func TestBridgeNamesBothRepositories(t *testing.T) {
	if sourceRepo != "racecraft-lab/typesafe-mcp" {
		t.Errorf("sourceRepo = %q", sourceRepo)
	}
	if releaseTagPrefix != "typesafe-jev-" {
		t.Errorf("releaseTagPrefix = %q", releaseTagPrefix)
	}

	var stdout, stderr bytes.Buffer
	cmd := newVersionCmd()
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs(nil)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	// The release pipeline compares stdout with the tag, so it holds the
	// version and nothing else.
	if got := stdout.String(); got != version+"\n" {
		t.Errorf("stdout = %q, want only the version", got)
	}
	// An operator who just updated from 0.8.0 is told there is one more hop.
	for _, want := range []string{githubRepo, "evaluate update"} {
		if !strings.Contains(stderr.String(), want) {
			t.Errorf("stderr %q does not name %q", stderr.String(), want)
		}
	}

	stdout.Reset()
	cmd = newVersionCmd()
	cmd.SetOut(&stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"--verbose"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"repository: " + sourceRepo, "updates:    " + githubRepo} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("verbose output %q lacks %q", stdout.String(), want)
		}
	}
}

// tarGz packs name/content pairs, in order, the way the release workflow in
// racecraft-plugins-public does: `tar -czf ... evaluate LICENSE`.
func tarGz(t *testing.T, files ...[2]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	for _, f := range files {
		if err := tw.WriteHeader(&tar.Header{Name: f[0], Mode: 0o755, Size: int64(len(f[1])), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(f[1])); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// BRIDGE-02: the whole hop a v0.8.1 install takes, short of replacing the
// running test binary. From a listing that interleaves other components, it
// selects the typesafe-jev-v* release, finds this platform's archive and the
// checksum file, verifies the archive, and extracts evaluate from the archive
// layout racecraft-plugins-public publishes, with LICENSE beside it.
func TestBridgeFollowsANewRepositoryRelease(t *testing.T) {
	archiveName := fmt.Sprintf("evaluate-%s-%s.tar.gz", runtime.GOOS, runtime.GOARCH)
	archive := tarGz(t, [2]string{"evaluate", "new binary"}, [2]string{"LICENSE", "MIT"})
	sum := sha256.Sum256(archive)
	sums := fmt.Sprintf("%s  %s\n%s  evaluate-plan9-mips.tar.gz\n", hex.EncodeToString(sum[:]), archiveName, strings.Repeat("0", 64))

	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	asset := func(name string) githubAsset {
		return githubAsset{Name: name, BrowserDownloadURL: srv.URL + "/download/" + name}
	}
	releases := []githubRelease{
		{TagName: "speckit-pro-v2.40.0", Assets: []githubAsset{asset("SHA256SUMS.txt")}},
		{TagName: "typesafe-jev-v0.9.1", Assets: []githubAsset{asset(archiveName), asset("SHA256SUMS.txt")}},
		{TagName: "typesafe-jev-v0.9.0"},
	}
	mux.HandleFunc("/repos/"+githubRepo+"/releases", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") != "1" {
			json.NewEncoder(w).Encode([]githubRelease{})
			return
		}
		json.NewEncoder(w).Encode(releases)
	})
	mux.HandleFunc("/repos/"+githubRepo+"/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		t.Error("the updater asked for the repository's latest release")
		http.NotFound(w, r)
	})
	mux.HandleFunc("/download/"+archiveName, func(w http.ResponseWriter, _ *http.Request) { w.Write(archive) })
	mux.HandleFunc("/download/SHA256SUMS.txt", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte(sums)) })
	withReleasesAPI(t, srv, 100)

	ctx := context.Background()
	release, latest, err := fetchLatestRelease(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if release.TagName != "typesafe-jev-v0.9.1" {
		t.Fatalf("picked %s, want typesafe-jev-v0.9.1", release.TagName)
	}
	bridge, _ := parseVersion("v0.8.1")
	if !latest.newerThan(bridge) {
		t.Fatalf("%s is not newer than the v0.8.1 bridge", release.TagName)
	}

	archiveURL, err := findAssetURL(release.Assets, archiveName)
	if err != nil {
		t.Fatal(err)
	}
	sumsURL, err := findAssetURL(release.Assets, "SHA256SUMS.txt")
	if err != nil {
		t.Fatal(err)
	}
	want, err := fetchExpectedChecksum(ctx, sumsURL, archiveName)
	if err != nil {
		t.Fatal(err)
	}
	got, err := download(ctx, archiveURL, maxArchiveBytes)
	if err != nil {
		t.Fatal(err)
	}
	if gotSum := sha256.Sum256(got); hex.EncodeToString(gotSum[:]) != want {
		t.Fatal("the archive does not match its SHA256SUMS.txt entry")
	}
	var bin bytes.Buffer
	if err := extractBinaryFromTar(got, "evaluate", &bin); err != nil {
		t.Fatal(err)
	}
	if bin.String() != "new binary" {
		t.Errorf("extracted %q, want the evaluate member", bin.String())
	}
}
