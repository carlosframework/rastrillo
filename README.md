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
  action, a `main.go` wiring `rastrillo.Serve`. Runs generate once so
  `go build` works immediately.
- **`rastrillo generate [dir]`** — the filesystem-routing generator
  (design doc §4): walks `actions/`, emits `gen/router.go` on a Go 1.22
  `http.ServeMux`. Fails loudly on route collisions.
- **`rastrillo.Serve`** — the bootstrap (design doc §5): the SQLite
  pragma-ordering fix, `SetMaxOpenConns(1)`, additive migrations; the
  platform's activation contract (`-socket`/`-addr`/systemd
  `LISTEN_FDS`, matching `carlosframework/platform`'s `testdata/echoapp`
  exactly); `GET /healthz` and `GET /api/version` answered automatically.
  **Covers only the plain "always-on instance" route kind** — a real
  gap found deploying hello world for real (below): the platform's
  hibernating instances also expect a `-db` flag (for the activator's
  restore/checkpoint cycle), and its `-backing unit` systemd tenants
  expect a `serve` subcommand plus inherited `LISTEN_FDS` socket
  activation, neither of which `rastrillo.Serve` implements yet.
- **`examples/helloworld`** — a real scaffolded app, checked in, proven
  to ship/promote/serve through the actual `carlos` binary — see
  [`hack/local-deploy-demo.sh`](hack/local-deploy-demo.sh).

**Not built yet** — all designed in the spec above, none of it faked or
stubbed here: the manifest system (`Resource`/`List`/`Form`, TOML sugar,
codegen-with-skip), `sqlc` query colocation, the `Mergeable` event-sourced
store shape, blobs, the crypto core, WebAuthn, the agents system, the
component/UI vocabulary, localization, `rastrillo dev`'s watch loop, and
the preloaded `CLAUDE.md`/skill scaffolding. Each is a real, separate
piece of work — see the design doc for the shape of each.

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
cd myapp && go mod tidy && go build ./... && ./myapp -addr :8080
```

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
(matching the flagship's `console.service` precedent), since this app
predates `-hibernate`/`-backing unit` support noted above. App hostnames
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
