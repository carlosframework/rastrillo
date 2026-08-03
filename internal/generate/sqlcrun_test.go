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
// (via the module's own tool directive), runs Task 5's emitted store
// through it, and builds the result. If the fetch fails for network
// reasons, this test skips rather than fails — Task 10 (blog adoption)
// exercises the identical path in CI, so a skip here is not silent
// coverage loss.
func TestRunSqlcGeneratesCompilingStore(t *testing.T) {
	root := newScratchModule(t, true)

	getCmd := exec.Command("go", "get", "-tool", "github.com/sqlc-dev/sqlc/cmd/sqlc")
	getCmd.Dir = root
	if out, err := getCmd.CombinedOutput(); err != nil {
		t.Skipf("go get -tool sqlc failed (likely a network issue): %v\n%s", err, out)
	}

	r := fixtureResource()
	if _, err := EmitStore(filepath.Join(root, "gen"), []rastrillo.Resource{r}); err != nil {
		t.Fatalf("EmitStore: %v", err)
	}

	if err := RunSqlc(root); err != nil {
		t.Fatalf("RunSqlc: %v", err)
	}

	// The point of this test over EmitStore's own golden tests (which
	// never invoke sqlc or the Go compiler): the emitted store must
	// actually compile once sqlc has generated Go from it.
	buildCmd := exec.Command("go", "build", "./...")
	buildCmd.Dir = root
	if out, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("go build ./... in scratch module: %v\n%s", err, out)
	}
}
