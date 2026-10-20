# AGENTS.md

Guidance for anyone changing Evigo, and for the coding tools they use. Read it
before making a change. [README.md](README.md) is the user documentation.

## What Evigo is

Evigo watches one UPS through Network UPS Tools (NUT). When the UPS goes on
battery, it remembers which devices were online. Once mains power is back,
stable, and the battery is charged enough, it wakes those devices with
Wake-on-LAN, retrying until each one answers ping or its timeout runs out.

Evigo only ever turns devices on. It never shuts anything down and never sends
UPS commands. Waking a device at the wrong moment is worse than not waking it,
so every change must keep the [safety rules](#safety-rules).

It is one Go binary with SQLite built in (pure Go, `CGO_ENABLED=0`), configured
by a single YAML file. It ships as static Linux amd64/arm64 binaries, a Docker
image, and a systemd unit with an installer. An optional web server, off by
default, serves a read-only status and history API and a health check.

## Commands

Run everything from the repository root. You need Go 1.26+, Node 24 (LTS) with
npm, ShellCheck, and Docker for container checks. Go tools are
pinned in the `Makefile` and run through `go run`; dashboard dependencies are
pinned in `web/package-lock.json`.

| Command                                                    | Use it to                                                                                                    |
| ---------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------ |
| `make format`                                              | Format Go code with gofumpt, and the dashboard with ESLint fixes and Prettier.                               |
| `make generate`                                            | Regenerate database code after editing `schema.sql` or `queries.sql`.                                        |
| `make test`                                                | Run the Go tests with the race detector.                                                                     |
| `make check`                                               | Run everything CI runs: formatting, generated code, Go and dashboard lint and types, scripts, and all tests. |
| `make security-check`                                      | Scan Go and npm dependencies for known vulnerabilities (needs network access).                               |
| `make build`                                               | Build the dashboard into `internal/api/static`, then `bin/evigo`.                                            |
| `go run ./cmd/evigo validate --config config.example.yaml` | Check the example configuration without touching a UPS or the network.                                       |
| `docker build --target test -t evigo:test .`               | Run vet and race tests on Linux.                                                                             |
| `docker build -t evigo:local .`                            | Build the runtime image without starting it.                                                                 |
| `make release`                                             | Build release archives, installer, and checksums into `dist/`. Publishes nothing.                            |

Focused checks: `make go-lint`, `make frontend-lint`, `make frontend-test`,
`make shell-check`, `make workflow-check`. Go tests don't need the dashboard
built. Run a single test while iterating:

```sh
go test -race ./internal/recovery/... -run TestGivenWakeSent
npm --prefix web test -- src/App.test.tsx
```

To run a local build with Docker:
`docker compose -f compose.yaml -f compose.build.yaml up -d --build`.

Before handing off a code change, run `make check`. If a check can't run in your
environment, say which one and why. Building images and archives is never the
same as starting or restarting a deployment.

## How the code fits together

Startup, in `cmd/evigo/main.go`, builds everything once:

```
config.Load         YAML → validated Config
devices.FromConfig  Config → []*Device (MAC parsed, wake address resolved)
database.Open       file checks, lock, schema check, LoadPending → saved progress
recovery.New        recovery.Settings + devices + saved progress
monitor.New         monitor.Settings + everything above
api.New().Serve     api.Settings + monitor.Snapshot + db.History, only if web.listen is set
monitor.Run         loops until stopped
```

Each loop pass, in `internal/monitor`, runs in a fixed order:

1. Read the UPS, or note that it couldn't be read.
2. Ping every device in parallel.
3. `recovery.Update` applies the rules and returns the devices to wake.
4. Save pending progress if it changed. If saving fails, stop here.
5. Recheck the UPS reading's age before each wake packet, bound its deadline by
   the reading's expiry, and record each send or failure in history.
6. Save the pass's history events, if there are any.
7. Replace the single status snapshot that the API reads.

Passes are scheduled from when the previous one started, so time spent inside a
pass does not lengthen the gap between them. A pass that overruns its interval is
followed straight away by the next; passes never overlap.

| Path                         | Responsibility                                                                                     |
| ---------------------------- | -------------------------------------------------------------------------------------------------- |
| `cmd/evigo/`                 | `run`, `validate`, `version`; builds everything and maps config values.                            |
| `internal/config/`           | Strict YAML loading, credentials, defaults, validation.                                            |
| `internal/devices/`          | A device's identity, ping check, and Wake-on-LAN packet.                                           |
| `internal/nut/`              | NUT protocol client, readings, and a reconnecting reader.                                          |
| `internal/recovery/`         | All recovery decisions. No I/O.                                                                    |
| `internal/monitor/`          | The loop, the order of I/O within each pass, and the latest snapshot.                              |
| `internal/snapshot/`         | What one pass saw: the snapshot type, its copying, and the power block.                            |
| `internal/api/`              | HTTP status and history API, Host/Origin protection, dashboard files.                              |
| `internal/database/`         | SQLite file checks, locking, schema checks, pending progress, history.                             |
| `internal/history/`          | History event types shared by recovery, the monitor, database and API.                             |
| `internal/database/queries/` | Generated by sqlc. Never edit by hand.                                                             |
| `deploy/`                    | Installer script and systemd unit.                                                                 |
| `.github/`                   | CI (Verify), the manual Release workflow, Dependabot, PR template.                                 |
| `web/`                       | The React dashboard. `make build` compiles it into `internal/api/static/`, which is not committed. |

Tests sit beside the code as `*_test.go`. `bin/` and `dist/` are build outputs.

## Architecture rules

- **Build once, at the boundary.** Turn configuration into finished objects at
  startup (`devices.FromConfig` parses the MAC and resolves the wake address).
  Don't re-derive configuration where it's used.
- **Only `cmd/evigo` and `devices.FromConfig` read `config.Config`.** Other
  packages define a small `Settings` struct holding exactly what they use, and
  `cmd/evigo` fills it in. The YAML layout stays out of the domain code.
- **Decisions in `internal/recovery`, I/O around it.** Recovery takes a `Pass`
  of observations and returns devices to wake; it never touches the network or
  disk. The monitor owns the order of reading, saving, and sending. Nothing else
  may bypass either.
- **Hand out copies, not internal state.** `Recovery.Pending()` returns value
  copies so the monitor can compare and save without sharing memory.
- **The API only reads.** It is given `monitor.Snapshot`, a function returning a
  copy of the single snapshot the monitor replaces after each pass. It never
  calls recovery or changes state. Whether waking is blocked, and
  why, comes from the pure `recovery.Settings.PowerBlock`, evaluated at request
  time so it matches what recovery itself would decide.
- **The API returns facts and codes, not wording.** Reasons are stable snake_case
  codes such as `stabilising` or `no_reading`; any user interface turns them into
  text.
- **Every network call has a deadline**, from a `context.Context`.
- **Errors and logs never leak secrets or untrusted text:** no credentials, raw
  NUT server messages, or YAML values.
- **Use typed values, not message matching.** If code must react to a particular
  failure, expose a type or code; don't inspect error text.
- **SQL lives in `.sql` files.** Queries are in `internal/database/queries.sql`
  and the schema in `internal/database/schema.sql`. Run `make generate` and
  commit the generated Go with them. A database is only opened if it has the
  current `schemaVersion`, passes SQLite's integrity check and its saved
  progress can be read; any other file is refused, never overwritten.
- **History only records changes.** Recovery reports its own transitions as
  events that the monitor collects; the monitor adds UPS status, reachability
  and wake results. A pass with no changes writes nothing.

## Safety rules

These protect real devices. Any change touching recovery, the loop, the
database, or packets must keep every one of them and add a regression test.

- Missing, malformed, contradictory, or unsafe UPS readings never allow a wake.
  Safe means `OL`, with only `CHRG`, `TRIM` or `BOOST` besides, and battery at or
  above the threshold.
- Power must stay safe for the full stability delay before any wake. A missing
  or expired reading, a restart, a storage failure, or a gap between passes longer
  than `nut.max_reading_age` starts the delay again. Backward time observations
  also reset the delay. Unobserved intervals never consume the device timeout.
- Only devices that answered ping on mains just before the outage are woken.
  Devices already off stay off, and a normal startup with nothing pending wakes
  nothing.
- Only an offline ping result triggers a wake; an unknown result never does.
- Devices are watched until waking is allowed. A device that answers ping then
  is done and is never woken, including one that shuts down later; a device
  still shutting down when mains returns is woken if it is off by then.
- Device timeouts count down only while waking is allowed.
- An attempt is saved before its packet is sent. If saving fails, nothing is
  sent. A packet is only sent while its UPS reading is still fresh.
- A restored attempt timestamp ahead of the clock never suppresses retries once
  the full stability delay has passed.
- Pending progress survives restarts and repeated outages. Damaged or unfamiliar
  database files are refused, never overwritten or discarded.
- Ping shows reachability, not application health, and a sent packet doesn't
  prove a boot; only a later ping reply does.
- The API keeps its Host and Origin checks whether or not a reverse proxy is in
  front of it.
- History is best effort: failing to save it never blocks an automatic wake.
  Unsaved events are kept, up to 1,000, and retried on the next pass.

## Code style

- Run `make format`. Follow the existing layout: imports separated from
  declarations, function bodies on multiple lines, a blank line between
  functions, and a blank line before a `return` that follows other work.
- Exported names are PascalCase, private names camelCase.
- **Names say completely what something is or does.** A function that does
  three things shouldn't be named after one. Distinguish configuration, saved
  state, observations and network results in variable names. Don't reuse UPS
  words like "charge" for recovery bookkeeping: `spendRecoveryTime`, not `charge`.
- **Don't add comments** unless they explain non-obvious reasoning, such as why
  error details are withheld. Prefer clear names and simple structure; never
  write comments that restate the code.
- Keep changes focused. No drive-by refactors or reformatting of untouched code.

## Testing

- Use the standard `testing` package. Don't add a BDD or assertion framework.
- Name every test `TestGiven<Situation>When<Action>Then<Outcome>`, and mark the
  body with `// Given`, `// When` and `// Then` comments.
- Dashboard tests use Vitest and Testing Library, nested as
  `describe('Given …')` → `describe('When …')` → `it('Then …')`, each starting
  with a capital. The Given names what is being exercised (for example "a request
  for Evigo's status"), each When names a condition it meets ("Evigo is
  available"), and each `it` checks one outcome. A When's `beforeEach` sets up its
  condition and then performs the Given.
- Use simulated UPS readings, a fake ping runner, loopback UDP listeners,
  temporary databases, and documentation addresses such as `192.0.2.10`.
  **Never** point tests at a real UPS or real devices.
- Assert observable outcomes (what was woken, saved, or refused), not
  implementation details.
- Add a focused regression for any change to recovery, reconnecting,
  persistence, or packets.

## Documentation

- Changes users would notice update `README.md`, and new or changed settings
  update `config.example.yaml`.
- Changes to recovery rules update the [safety rules](#safety-rules) here.
- User-visible changes get a line in `CHANGELOG.md`.
- Keep examples generic: no real hostnames, IPs, MACs, or credentials.

## Pull requests and releases

- Fill in `.github/pull_request_template.md`: the problem, the resulting
  behaviour, and the checks you ran.
- `make check` must pass. Dependency changes also need `make security-check` and
  updated notices in the single root `THIRD_PARTY_LICENSES`.
- Changes to `deploy/`, the `Dockerfile`, or workflows also need `docker build`,
  `make release`, and the archives checked.
- GitHub Actions are pinned to version tags, not commit SHAs, and each check is
  a separate CI job so failures report independently. Keep it that way.
- Deployment, release, and `VERSION` changes need maintainer review
  (`.github/CODEOWNERS`).
- `VERSION` is the release version. The `main` ruleset only lets a pull request
  merge into main once it is up to date with main and every Verify job passes.
  Nothing is published on merge. To release, a maintainer updates `VERSION` and
  `CHANGELOG.md`, merges to main, and runs **Release** in GitHub Actions. It re-runs every check
  and publishes the Linux archives, installer, `SHA256SUMS`, and
  `ghcr.io/jack-overflow/evigo` images that were verified.

## Boundaries

**Always:** keep the safety rules, keep examples generic, add regressions for
behaviour changes, update the docs, and fix findings instead of weakening checks.

**Ask the maintainer first:**

- publishing, pushing, tagging, or running the Release workflow; changing GitHub
  or package settings,
- starting or restarting a live deployment, changing local deployment
  configuration, or deleting persistent recovery data,
- changing configuration keys, database compatibility, or when devices are
  woken beyond what the change was for,
- adding dependencies.

**Never:**

- commit or expose credentials or private deployment data (`.env`,
  `config.yaml`, `compose.override.yaml`, database files),
- use a live UPS or real devices as test targets,
- overwrite unrelated changes in the working tree,
- hand-edit generated code, dependencies, or build outputs.
