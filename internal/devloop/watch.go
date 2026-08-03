// Package devloop implements the polling file watcher behind `rastrillo
// dev` (design doc §11). Polling rather than a notify dependency, per
// the family's hand-roll-page-of-code convention (cmd/rastrillo/
// modpath.go): portable, zero deps, and a 250ms poll of a small source
// tree sits far inside §11's ~2s save-to-serving target.
package devloop

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Stamp fingerprints one file: size plus mtime is enough to detect an
// edit without hashing content. Known limit: on filesystems with coarse
// (e.g. 1s) mtime granularity, a same-size edit landing inside the same
// tick as a previous stat can be missed until the next change; ext4 and
// APFS give mtimes nanosecond granularity, so this is a non-issue on dev
// machines.
type Stamp struct {
	Size    int64
	ModTime time.Time
}

// Snapshot fingerprints every regular file under each root/dirs entry,
// keyed by slash-separated path relative to root. Missing dirs are
// skipped, not errors: a fresh `rastrillo new` app has no app/ or
// manifest/ yet. Files vanishing mid-walk are skipped for the same
// reason — the next poll settles it.
func Snapshot(root string, dirs []string) (map[string]Stamp, error) {
	snap := map[string]Stamp{}
	for _, d := range dirs {
		base := filepath.Join(root, d)
		if _, err := os.Stat(base); os.IsNotExist(err) {
			continue
		}
		err := filepath.WalkDir(base, func(path string, de fs.DirEntry, err error) error {
			if err != nil {
				if os.IsNotExist(err) {
					return nil
				}
				return err
			}
			if de.IsDir() {
				return nil
			}
			info, err := de.Info()
			if err != nil {
				if os.IsNotExist(err) {
					return nil
				}
				return err
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			snap[filepath.ToSlash(rel)] = Stamp{Size: info.Size(), ModTime: info.ModTime()}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return snap, nil
}

// Diff returns the sorted set of paths added, modified, or removed
// between two snapshots.
func Diff(prev, next map[string]Stamp) []string {
	changed := map[string]bool{}
	for p, s := range next {
		if ps, ok := prev[p]; !ok || ps != s {
			changed[p] = true
		}
	}
	for p := range prev {
		if _, ok := next[p]; !ok {
			changed[p] = true
		}
	}
	out := make([]string, 0, len(changed))
	for p := range changed {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}
