package pgo

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/pprof/profile"
)

// doNotInlineFunc is a function the noinline hack is expected to rename when
// it appears as a leaf in the testdata profile.
const doNotInlineFunc = "google.golang.org/grpc/internal/transport.(*loopyWriter).processData"

// TestMergedProfilePipeline exercises the same code path the datadog-pgo CLI
// drives against the Datadog gopgo endpoint: it packages the bundled testdata
// profile as the ZIP the endpoint returns, then runs
// ProfilesDownload.MergedProfile -> ApplyNoInlineHack -> Write, and asserts the
// result is a valid, deterministic .pgo file with the noinline hack applied.
//
// It needs no network access and no Datadog credentials.
func TestMergedProfilePipeline(t *testing.T) {
	profBytes, err := os.ReadFile(filepath.Join("testdata", "grpc-anon.pprof"))
	if err != nil {
		t.Fatalf("read testdata: %v", err)
	}

	// gopgoEndpointZIP packages two copies of the test profile as the ZIP the
	// /api/unstable/profiles/gopgo endpoint returns. Each entry is a separate
	// pprof stream that MergedProfile parses and merges.
	gopgoEndpointZIP := func() []byte {
		var buf bytes.Buffer
		zw := zip.NewWriter(&buf)
		for _, name := range []string{"profile-0.pprof", "profile-1.pprof"} {
			w, err := zw.Create(name)
			if err != nil {
				t.Fatalf("create zip entry %s: %v", name, err)
			}
			if _, err := w.Write(profBytes); err != nil {
				t.Fatalf("write zip entry %s: %v", name, err)
			}
		}
		if err := zw.Close(); err != nil {
			t.Fatalf("close zip: %v", err)
		}
		return buf.Bytes()
	}

	// runPipeline mirrors the CLI's Fetch end-to-end for the merge/write stage.
	runPipeline := func(dst string) (n int64, samples int, sha string) {
		log := slog.New(slog.NewTextHandler(io.Discard, nil))
		dl := &ProfilesDownload{data: gopgoEndpointZIP()}
		mp, err := dl.MergedProfile(log)
		if err != nil {
			t.Fatalf("MergedProfile: %v", err)
		}
		if err := mp.ApplyNoInlineHack(); err != nil {
			t.Fatalf("ApplyNoInlineHack: %v", err)
		}
		n, err = mp.Write(dst)
		if err != nil {
			t.Fatalf("Write: %v", err)
		}
		samples = mp.Samples()
		data, err := os.ReadFile(dst)
		if err != nil {
			t.Fatalf("read %s: %v", dst, err)
		}
		sum := sha256.Sum256(data)
		sha = string(sum[:])
		return n, samples, sha
	}

	dir := t.TempDir()
	out1 := filepath.Join(dir, "default.pgo")
	n, samples, sha1 := runPipeline(out1)

	// The merge must have produced a non-trivial profile.
	if samples <= 0 {
		t.Fatalf("merged profile has no samples; got %d", samples)
	}
	if n <= 0 {
		t.Fatalf("wrote no bytes; got %d", n)
	}

	// The output must be a valid pprof the Go toolchain can consume.
	parsed, err := profile.Parse(bytes.NewReader(mustRead(t, out1)))
	if err != nil {
		t.Fatalf("output is not a parseable pprof: %v", err)
	}
	if len(parsed.Sample) != samples {
		t.Fatalf("parsed sample count %d != merged %d", len(parsed.Sample), samples)
	}

	// The noinline hack must have renamed the target leaf function. The
	// testdata profile is known to contain doNotInlineFunc as a leaf, so after
	// ApplyNoInlineHack no function may still bear that exact name and at
	// least one must carry the DO NOT INLINE prefix.
	var hasRenamed, hasUnrenamed bool
	for _, f := range parsed.Function {
		switch f.Name {
		case doNotInlineFunc:
			hasUnrenamed = true
		case doNotInlinePrefix + doNotInlineFunc:
			hasRenamed = true
		}
	}
	if hasUnrenamed {
		t.Errorf("noinline hack did not rename %q (still present unmodified)", doNotInlineFunc)
	}
	if !hasRenamed {
		t.Errorf("noinline hack did not produce %q%s", doNotInlinePrefix, doNotInlineFunc)
	}

	// The pipeline must be deterministic: a second run yields byte-identical
	// output. This guards against accidental non-determinism (map iteration
	// order, timestamps, etc.) that would break reproducible PGO builds.
	out2 := filepath.Join(dir, "default.pgo.again")
	_, samples2, sha2 := runPipeline(out2)
	if samples != samples2 {
		t.Errorf("non-deterministic sample count: %d vs %d", samples, samples2)
	}
	if sha1 != sha2 {
		t.Errorf("non-deterministic output: sha256 differs between runs")
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return b
}
