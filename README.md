# 🤖 Rastrillo

The CARLOS web framework — the shape of a CARLOS app, the way the platform
(`carlosframework/platform`) is the shape of the deployment substrate it
runs on. Read the full design at
[`carlosframework/platform`'s spec library](https://github.com/carlosframework/platform/blob/main/docs/superpowers/specs/2026-08-01-carlos-framework-design.md)
(approved and merged 2026-08-01).

## Status: v1 walking skeleton

This is a first pass, built overnight to prove the core loop end to end
rather than to cover the full design. **Built:**

- **`rastrillo new <name>`** — scaffolds a Go app: `go.mod`, one starter
  action, a `main.go` wiring `rastrillo.Run`. Runs generate once so
  `go build` works immediately.
- **`rastrillo generate [dir]`** — the filesystem-routing generator
  (design doc §4): walks `actions/`, emits `gen/router.go` on a Go 1.22
  `http.ServeMux`. Fails loudly on route collisions. Action files carry
  `//go:build rastrillo_actions` (scaffolded for you; stripped from the
  compiled copies under `gen/`) so `go build ./...`, `go vet ./...` and
  `go test ./...` skip generator input instead of failing on it —
  `generate --check` names any file missing the constraint.
- **`rastrillo dev [dir] [-- app args]`** — the development watch loop
  (design doc §11): watches `app/`, `actions/`, `manifest/`, `cmd/`,
  `locales/`, and `templates/` by polling. On any change, reruns `rastrillo
  generate`, builds the app's `./cmd/<name>` package to a temporary binary
  (cleaned up on exit), and restarts the running process (graceful
  SIGTERM). A failed generate or rebuild keeps the previous build serving;
  a failed restart keeps the loop watching too — either way, the next save
  retries. Expects the `rastrillo new` layout: exactly one directory under
  `cmd/`. Useful for rapid iteration: edits to `actions/` require
  regeneration (the binary uses generated code under `gen/`), and `dev`
  does that automatically.
- **`rastrillo.Run`** — the process entrypoint the scaffold wires up: it
  resolves whichever of the platform's two activation argv shapes the
  binary was invoked with — `-socket`/`-addr`/`-db` flags for an agent
  exec child (hibernate routes), or a bare `serve` subcommand with no
  flags for a `carlos-app@.service` unit tenant — then calls `Serve`. A
  relative `-db`/`Options.DBPath` is resolved inside `$STATE_DIRECTORY`
  when systemd provides one, since a unit tenant's cwd isn't its state
  dir. Hibernation requires nothing else from the app: the activator
  owns the restore/replicate cycle, and `Serve`'s SIGTERM drain fits
  inside its SIGKILL budget. `rastrillo.Resolve` is the same resolution
  without the serving, for apps that need the resolved invocation before
  doing anything else with it — e.g. one that wants the resolved
  `DBPath` without going through `Options.Router`.
- **`rastrillo.Serve`** — the bootstrap (design doc §5): the SQLite
  pragma-ordering fix, `SetMaxOpenConns(1)`, additive migrations; the
  platform's activation contract (`Options.Socket`/`Options.Addr`/systemd
  `LISTEN_FDS`, matching `carlosframework/platform`'s `testdata/echoapp`
  exactly); `GET /healthz` and `GET /api/version` answered automatically.
  An app that keeps its database in `Ctx` sets `Options.Router` instead
  of `Options.Mux` and is handed the `*sql.DB` Serve opened;
  `rastrillo.OpenDB` is the same corrected opener exported for tests.
  Between `Serve` and `Run`, the activation contract is covered end to
  end: every route kind the platform runs — always-on instance,
  hibernating exec child, unit tenant — boots the same scaffolded app.
- **Localization** (design doc §10) — `Options.Locales`/`DefaultLocale`/
  `LocaleFS` declare an app's locale set and supply its catalogs from an
  `embed.FS` carrying `locales/<code>.toml` (flat `key = "value"` TOML).
  Each request resolves a locale in order: URL path prefix (stripped
  before the app's mux sees it, so `/fr/orders` and `/orders` reach the
  same route), then `Accept-Language` (q-ordered, so a browser sending
  `fr-CA` matches a declared `fr`), then the `rastrillo_locale` cookie,
  then the default. Actions call request-scoped `rastrillo.T(r, key)` /
  `Tf(r, key, args...)` (`{name}` interpolation) for translated strings;
  lookup falls back through the requested locale's catalog, the default
  locale's catalog, the framework's base catalog, and finally the key
  itself — a missing translation stays visible on the page, never blank.
  The framework base catalog (`rastrillo.BaseCatalog()`, wired into every
  `Serve`d app's `Locales` automatically) carries `rastrillo/ui`'s own
  `rastrillo.ui.*` strings, so a single-locale app gets correctly-worded
  built-in components without writing a catalog of its own; an app
  catalog entry for the same key still wins. `rastrillo generate --check [--default-locale
  <code>]` fails loudly when a non-default catalog is missing keys the
  default has (§10's "silent fallback while iterating, loud failure
  before ship"); that gate runs under `--check` only — plain `rastrillo
  generate` (and so `rastrillo dev` and `rastrillo new`) never fails on an
  incomplete catalog. `--check`'s `--default-locale` flag defaults to
  `en` and is not read from `Options.DefaultLocale` — if an app sets a
  different `DefaultLocale`, pass the matching `--default-locale` by hand
  or the check compares against the wrong catalog. Nothing writes the
  `rastrillo_locale` cookie yet; persisting a user's locale choice across
  requests is the app's job for now. Two honest caveats: an app that
  declares locale `en` can't serve an app route whose first path segment
  is also `en` — inherent to prefix routing, not a bug to fix; and a
  `ServeMux` trailing-slash redirect issued under a locale prefix
  currently emits the unprefixed path, dropping the locale on that one
  redirect (known limitation).
- **`rastrillo/ui`** — the component/UI vocabulary (design doc's List
  screens plus the display, form, and route families): badges, meters,
  person cells, callouts, fields, choice cards, toggle blocks, seg-tabs,
  confirm forms, bulk select, modal shells, and the rest of the List
  screen set — with framework strings resolved through the §10 locale
  chain. An app registers `ui.Funcs()` (`dict`, `list`, `icon`, `T`) on
  its own template tree and `ParseFS`s `ui.Templates()` alongside its own
  templates; `ui.TokensCSS()` is the design-token stylesheet
  `rastrillo new` writes once into a new app's `static/` directory, app-
  owned from then on. `T` resolves a partial's own hardcoded-English
  default (e.g. `pagination`'s "Pagination", `confirm-form`'s "Cancel")
  through the framework base catalog — a caller-supplied value always
  wins over it — and `ui.FuncsWith` lets an app rebind `T` to a
  request-scoped `rastrillo.T` lookup so those defaults resolve in the
  request's locale instead. See `ui`'s package doc for the full class
  idiom vocabulary (list grid, dropdown, filter tokens, help tooltip,
  selection checkbox) that isn't a Go template partial.
- **`examples/helloworld`** — a real scaffolded app, checked in, proven
  to ship/promote/serve through the actual `carlos` binary — see
  [`hack/local-deploy-demo.sh`](hack/local-deploy-demo.sh).

**Not built yet** — all designed in the spec above, none of it faked or
stubbed here: the manifest system (`Resource`/`List`/`Form`, TOML sugar,
codegen-with-skip), `sqlc` query colocation, the `Mergeable` event-sourced
store shape, blobs, the crypto core, WebAuthn, the agents system, and the
preloaded `CLAUDE.md`/skill scaffolding. Each is a real, separate piece
of work — see the design doc for the shape of each.

## A known implementation decision worth flagging

The design doc's routing example puts multiple action files in the same
directory (`actions/orders/[id]/cancel.POST.go` next to `edit.GET.go`) —
but Go compiles one package per directory, so both files sharing a bare
`func Handle` would collide. The generator resolves this by never
compiling `actions/` in place: each file is parsed, its package clause
rewritten to a name unique to its route, and the result written into its
own directory under `gen/actions/`. Normal Go imports, no AST surgery
beyond the package clause. A future version could instead lift just the
`Handle` function via full AST extraction to get closer to "one file, no
package boilerplate," but that's real complexity, deliberately deferred
rather than rushed — see `internal/generate/generate.go`'s package doc.

## Try it

```
go install github.com/carlosframework/rastrillo/cmd/rastrillo@latest
rastrillo new myapp
cd myapp && go mod tidy && rastrillo dev
```

Then edit an action, save, refresh — `rastrillo dev` regenerates,
rebuilds, and restarts for you. For a one-off build without the watch
loop: `go build ./cmd/myapp && ./myapp -addr :8080`.

Or via Homebrew: `brew install carlosframework/tap/rastrillo`.

To see it actually deployed through the real platform binary (local
directory store + local registry + `carlos edge -dev`, no AWS/SSH
required):

```
PLATFORM_REPO=/path/to/carlosframework/platform hack/local-deploy-demo.sh
```

## Live

[`https://helloworld.dev.oncarlos.com`](https://helloworld.dev.oncarlos.com) —
the v1 walking skeleton's hello world, deployed for real on the
platform-dev environment: a real S3-backed deployment bucket, a real
`carlos edge`, a real Let's Encrypt certificate — not the local-directory
demo above. See `carlosframework/platform`'s
`docs/superpowers/specs/2026-08-02-platform-dev-environment-design.md`.
Routed as a plain always-on instance via a hand-written systemd unit
(matching the flagship's `console.service` precedent): `rastrillo.Run`
and its hibernate/unit-tenant support landed 2026-08-03, after this
deploy, so the live instance still runs the older hand-wired
`-socket`/`-addr` `main.go`, not `Run`. App hostnames
live under `oncarlos.com`, not `carlosframework.com` — that's reserved
for platform surfaces, e.g. the dev console itself at
[`https://platform.dev.carlosframework.com`](https://platform.dev.carlosframework.com)
(sign in with Keymail to see it and every other app on this deployment).

## See also

[carlosframework.com](https://carlosframework.com) for the architecture
rastrillo builds apps on top of, and
[`carlosframework/skills`](https://github.com/carlosframework/skills) for
the Claude Code skill capturing the family's conventions — including,
after this framework's first pieces landed, which of those conventions
rastrillo now enforces mechanically rather than asks you to remember.
