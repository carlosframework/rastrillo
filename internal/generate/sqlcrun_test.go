package generate

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/carlosframework/rastrillo"
)

// repoRoot returns this repo's absolute root, computed from this
// file's own location rather than the test's working directory, so
// it's stable regardless of how `go test` is invoked. (Same pattern as
// internal/manifest/goeval_test.go's helper of the same name — each
// package needs its own since it's unexported.)
func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root, err := filepath.Abs(filepath.Join(filepath.Dir(file), "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

// newScratchModule builds a standalone module in t.TempDir() that
// requires and replaces github.com/carlosframework/rastrillo with this
// repo, so RunSqlc and the compiled store it produces have a real
// module to work in. withTool controls whether the go.mod carries the
// sqlc tool directive from the start.
func newScratchModule(t *testing.T, withTool bool) string {
	t.Helper()
	root := t.TempDir()
	goMod := "module scratch\n\ngo 1.25.0\n\n"
	if withTool {
		goMod += "tool github.com/sqlc-dev/sqlc/cmd/sqlc\n\n"
	}
	goMod += "require github.com/carlosframework/rastrillo v0.0.0\n\n" +
		"replace github.com/carlosframework/rastrillo => " + repoRoot(t) + "\n"
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte(goMod), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestRunSqlcMissingToolSaysHowToAddIt(t *testing.T) {
	root := newScratchModule(t, false)

	err := RunSqlc(root)
	if err == nil {
		t.Fatal("RunSqlc: want error for a module without the sqlc tool directive, got nil")
	}
	want := "go get -tool github.com/sqlc-dev/sqlc/cmd/sqlc"
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error = %q, want it to contain %q", err, want)
	}
}

// TestRunSqlcGeneratesCompilingStore is the slice's heaviest
// integration test: it fetches the real sqlc binary over the network
// (via the module's own tool directive), runs the store emitter's
// output for TWO resource shapes through it, emits Task 8's actions
// against the same real sqlc output, and builds the whole module. If
// the fetch fails for network reasons, this test skips rather than
// fails — Task 10 (blog adoption) exercises the identical path in CI,
// so a skip here is not silent coverage loss.
//
// Two fixtures, not one, on purpose: fixtureResource ("notes") has
// both List.Search and a List.Filter, so its Count/List queries always
// have at least one bind parameter and sqlc always generates a
// Params struct for them. noAdvancedFixtureResource ("widgets") has
// neither — its Count query has ZERO bind parameters, and sqlc's own
// convention (discovered the hard way during Task 8's self-review,
// see task-8-report.md) is to drop the Params argument from the
// generated method entirely rather than emit an empty struct type.
// EmitActions' index.GET builder has to match that exactly or the
// generated action fails to compile against the real store — a
// mismatch a hand-written stub can hide (it did, briefly) but this
// real-sqlc round trip cannot: if a future sqlc version changes this
// convention, this test — not just TestEmitActionsCompile's stub —
// will fail loudly.
func TestRunSqlcGeneratesCompilingStore(t *testing.T) {
	root := newScratchModule(t, true)

	getCmd := exec.Command("go", "get", "-tool", "github.com/sqlc-dev/sqlc/cmd/sqlc")
	getCmd.Dir = root
	if out, err := getCmd.CombinedOutput(); err != nil {
		t.Skipf("go get -tool sqlc failed (likely a network issue): %v\n%s", err, out)
	}

	genDir := filepath.Join(root, "gen")
	notes := fixtureResource()
	widgets := noAdvancedFixtureResource()
	if _, err := EmitStore(genDir, []rastrillo.Resource{notes, widgets}); err != nil {
		t.Fatalf("EmitStore: %v", err)
	}

	if err := RunSqlc(root); err != nil {
		t.Fatalf("RunSqlc: %v", err)
	}

	if _, _, err := EmitActions(root, genDir, notes); err != nil {
		t.Fatalf("EmitActions(notes): %v", err)
	}
	if _, _, err := EmitActions(root, genDir, widgets); err != nil {
		t.Fatalf("EmitActions(widgets): %v", err)
	}

	// The point of this test over EmitStore/EmitActions' own golden
	// tests (which never invoke sqlc or the Go compiler): the emitted
	// store AND the emitted actions must actually compile together
	// once sqlc has generated real Go from the store side.
	buildCmd := exec.Command("go", "build", "./...")
	buildCmd.Dir = root
	if out, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("go build ./... in scratch module: %v\n%s", err, out)
	}
}
