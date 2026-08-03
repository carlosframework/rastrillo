# examples/blog — a whole app, built from stock parts

A single-user blog: a public reading side (index, one post per page) and
an admin section for writing, editing, publishing and deleting. Ten
actions on rastrillo's filesystem router, one SQLite table, the eight
List-screen partials this app itself uses (of the larger set `ui` now
ships — see `ui`'s package doc for the full, current list) each in its
intended role, and **no JavaScript** — no `<script>` tag anywhere, no
off-origin reference on any page. The look is `static/tokens.css` unedited plus about fifty app-owned
lines in `static/blog.css`.

`examples/helloworld` proves the framework's plumbing (scaffold,
generate, serve, deploy). This proves its *shape*: that an app assembled
from stock parts, with no JavaScript and almost no hand-written CSS, is a
thing a person would ship.

## Running it

```
cd examples/blog
go build ./cmd/blog
./blog -addr :8080
```

Then read at <http://localhost:8080/> and write at
<http://localhost:8080/admin/posts>.

**Run it from the app root.** Static files are served with
`http.FileServer(http.Dir("static"))`, which is relative to the working
directory, so starting the binary from anywhere else 404s both
stylesheets and every screen renders unstyled (F8).

**No `go generate` step.** `gen/` is committed, exactly as helloworld's
is, so `go build ./cmd/blog` works on a fresh clone.

**There is no auth.** The whole `/admin/…` subtree is open. Rastrillo's
actor and session layers are designed but unbuilt (see the repo README's
"Not built yet"), so this example runs locally and says so rather than
inventing a password field (F7).

## The commands that work

```
go build ./cmd/blog     # the binary (./... would discard it — go help build)
go test ./...
go vet ./...
```

`./...` works because every action file carries `//go:build
rastrillo_actions` — the F9 fix. Without the constraint, `go build
./...`, `go vet ./...` and `go test ./...` all failed here: actions/ is
generator input the go tool tried to compile anyway. See F9 below for
the original finding and its resolution.

## How it is put together

- `actions/` — one file per route, named `<name>.<VERB>.go`, in
  directories that spell the URL. It is generator *input*, not a
  compilable package.
- `gen/` — committed generator output. Never hand-edited.
- `internal/blog/store.go` — `Open`, the migration, `Post`, every query.
  Plain functions over `*sql.DB`, so `Ctx.DB` flows straight from an
  action into a query with nothing in between.
- `internal/blog/view.go` — the template tree, `Render`, `Fail`, and
  every view model. One base tree (ui's partials + the layout) cloned
  once per page at startup, because every page file defines `content`.
- `internal/blog/templates/` — `layout.html` and one file per screen.
- `internal/blogtest/` — the tests, a directory of `_test.go` files that
  drives the real generated mux through `httptest`.
- `static/` — `tokens.css` (shipped by `rastrillo new`, unedited) and
  `blog.css` (the app's own).

## Development

Regenerate after adding, renaming or removing an action:

```
go run github.com/carlosframework/rastrillo/cmd/rastrillo generate .
```

Check a committed `gen/` is fresh (for CI, or before review):

```
tmp=$(mktemp -d) && cp -R actions go.mod "$tmp"/
go run github.com/carlosframework/rastrillo/cmd/rastrillo generate "$tmp"
diff -r gen "$tmp/gen"
```

Copying `go.mod` is what makes the generated import paths match; the
`actions/` tree and that one file are the generator's whole input. This
is a procedure rather than a Go test because `internal/generate` is
internal to the rastrillo module and the CLI always writes to
`<dir>/gen`, so it cannot be pointed at a scratch output directory from
inside a test. The everyday guard against a stale `gen/` is the route
tests, which run through the committed router and fail immediately if it
no longer matches the actions.

## What building this revealed

Recorded, not fixed. Nothing outside `examples/blog/` changed on this
branch; each of these belongs to a later framework slice.

**F1 — `list-row-action` has no status slot.** A blog list's most
load-bearing fact per row is "draft or published", and the row contract
offers `Main`, `Sub`, one action pill and a decorative `aria-hidden`
lead marker. Status goes into `Sub` as prose. It works and it is
accessible; the row partial wants a `Status` `Tone`/`Label` pair that
renders `status-pill` in the row's right-hand group.

**F2 — no form partials, and the focus ring stops at the library's
edge.** The `field`/`field-textarea` family is deferred, so every form
here is hand-rolled, and `tokens.css` scopes its `:focus-visible` rule to
library containers, so `blog.css` has to restate the outline for controls
outside them. Roughly half of `blog.css` would disappear the day form
partials land.

**F3 — no `dropdown`, so "show drafts only" is missing.** Also deferred.
The obvious next control on the admin list — a status filter — has
nowhere to attach, so the example ships without it rather than
hand-rolling a control the library has already scheduled.

**F4 — `Serve`'s `DBPath`/`Migrations` are unusable by any app that puts
the DB in `Ctx`.** `Serve` opens the handle, defers its `Close`, and
never exposes it; `openDB` is unexported and `Options` carries no `DB`
field and no Ctx factory. So this app opens its own handle and
hand-copies the pragma DSN — `busy_timeout` before `journal_mode(WAL)`,
then `SetMaxOpenConns(1)` — which is precisely the hand-propagation the
framework exists to end. Next slice: return the handle from `Serve`, or
let `Options` carry the Ctx factory.
*Eased, not fixed:* `rastrillo.Resolve` now applies the activation
contract without serving, so this app honors `-db`, `serve`, and
`$STATE_DIRECTORY` while still opening its own handle (see
`cmd/blog/main.go`). The hand-copied pragma DSN remains — the real fix
is still the next-slice shape above.
*Fixed:* `Options.Router` now receives the `*sql.DB` that `Serve`
opened — pragmas, eager ping, and `Options.Migrations` applied — so
`cmd/blog/main.go` is back to plain `Run` and the hand-copied DSN is
gone. `rastrillo.OpenDB` is the same opener exported, which is what
`blog.Open` (kept for the tests) now wraps.

**F5 — `actions/` cannot hold shared code, by two rules.** The generator
copies only files matching `<name>.<VERB>.go`, and separately skips any
file whose base name starts with `_`. So neither `helpers.go` nor
`_helpers.go` ever reaches `gen/`, and an action calling into one
compiles in `actions/` and fails in `gen/`. Shared code lives in a normal
package (`internal/blog`); that is the only path, not a preference.

**F6 — `actions/index.GET.go` is a catch-all.** `index` maps to `GET /`
and Go's mux treats `/` as a prefix, so every unmatched GET lands in the
index action. Every app needs the `r.URL.Path != "/"` guard, or the
generator should emit a 404 fallback for it.

**F7 — no auth, because there is none to use.** The `/admin/…` subtree is
open. A real deployment would put it behind an authenticated session and
set `Ctx.Actor` from it instead of this app's hardcoded human actor;
nothing else about the app would change shape.

**F8 — static serving is relative to the working directory.** The
scaffolded line is `http.FileServer(http.Dir("static"))`, so the binary
must be run from the app root or every screen renders unstyled.

**F9 — `go build ./...`, `go vet ./...` and `go test ./...` all fail in a
rastrillo app.** `actions/` is generator input, not a package Go can
compile: two actions in one directory both declare `Handle`
(`actions/admin/posts/` has three), and a `[id]` directory is a malformed
Go import path — which is what every `./...` invocation trips over first,
`vet` included. helloworld never hit this because it has one action in
one directory. The working commands are listed above; the framework
should say so in `rastrillo new`'s output, and could remove the problem
entirely by reading actions from a directory the Go tool ignores.
*Fixed:* action files now carry `//go:build rastrillo_actions` — never
satisfied by a normal build, so every `./...` invocation skips
generator input (including the `[id]` directories) instead of failing
on it. `rastrillo new` scaffolds the constraint, `rastrillo generate`
strips it from the `gen/` copies, and `generate --check` fails with the
exact line to add when a file lacks it. This app is tagged; its
`./...` commands pass.

**F10 — `tokens.css` still styles a pagination state the partial no
longer emits.** `ui/partials/pagination.html` renders a disabled item as
a bare `<span>{{.Label}}</span>` — `aria-disabled` was deliberately
dropped in `c00653c` — but `tokens.css` still carries
`.rst-pagination [aria-disabled="true"] { border-style: dashed; color: var(--rst-text-faint) }`,
which now matches nothing. The visible result on this blog's list screens
is that a disabled `Previous` on page 1 looks identical to a live page
link: same border, same colour, differing only in not being clickable.
The fix belongs in the library — either restore the attribute on the
span, or restyle the rule to target the disabled item as the partial
actually emits it — and not in an example, so this branch changes
nothing.
*Fixed:* in the library, the second way — the span now carries
`class="rst-pagination__disabled"` and tokens.css styles that class.
`aria-disabled` stays dropped on purpose: the attribute belongs on
elements with an interactive role, and a bare span has none.
