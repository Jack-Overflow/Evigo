# AGENTS.md

Guidance for anyone changing Evigo, and for the coding tools they use.
[README.md](README.md) is the user documentation.

## What Evigo is

Evigo watches one UPS through Network UPS Tools (NUT). When the UPS goes on
battery, it notes which devices were online. Once mains power is back, stable,
and the battery is charged enough, it wakes the ones that are off with
Wake-on-LAN, retrying until each answers ping or its timeout runs out. An
optional read-only dashboard and API show what it is doing.

Evigo only ever turns devices on. It never shuts anything down and never sends
UPS commands. Waking a device at the wrong moment is worse than not waking it,
so every change must keep the [safety rules](#safety-rules).

## Commands

Run from the repository root. You need Go 1.26+, Node 24 (LTS) with npm,
ShellCheck, and Docker for container checks.

| Command                                                    | Use it to                                                                         |
| ---------------------------------------------------------- | --------------------------------------------------------------------------------- |
| `make check`                                               | Run everything CI runs. Must pass before handing off a change.                    |
| `make format`                                              | Format Go and the dashboard.                                                      |
| `make generate`                                            | Regenerate database code after editing `schema.sql` or `queries.sql`.             |
| `make build`                                               | Build the dashboard and `bin/evigo`.                                              |
| `make security-check`                                      | Scan dependencies for known vulnerabilities (needs network access).               |
| `go run ./cmd/evigo validate --config config.example.yaml` | Check a configuration without touching a UPS or the network.                      |
| `make release`                                             | Build release archives, installer, and checksums into `dist/`. Publishes nothing. |

Run a single test while iterating:

```sh
go test -race ./internal/recovery/... -run TestGivenWakeSent
npm --prefix web test -- src/App.test.tsx
```

If a check can't run in your environment, say which one and why. Building
images and archives is never the same as starting or restarting a deployment.

## Design principles

- **Recovery decides; everything else does I/O.** `internal/recovery` makes
  every recovery decision and never touches the network or disk. The monitor
  owns the order of reading, saving, and sending. Nothing bypasses either.
- **Build from configuration once, at startup.** Only `cmd/evigo` and
  `devices.FromConfig` read the configuration; other packages receive a small
  settings struct.
- **The API only reads and reports facts.** It formats the monitor's latest
  snapshot, returns stable codes such as `stabilising` rather than wording, and
  never changes state.
- **The dashboard only displays.** It turns codes into words, counts down to
  timestamps the API provides, and marks data outdated when Evigo stops
  answering. It never re-implements power, eligibility, or timing rules; if it
  needs a decision, add it to the API.
- **Every network call has a deadline.** Errors and logs never include
  credentials, raw NUT messages, or YAML values.
- **React to typed values and codes, never to message text.**
- **History only records changes.** A pass where nothing changed writes nothing.

## Safety rules

These protect real devices. A change touching recovery, the monitor loop, the
database, or packets must keep every one and add a regression test.

- Missing, malformed, or unsafe UPS readings never allow a wake. Safe means
  `OL`, with only `CHRG`, `TRIM` or `BOOST` besides, and battery at or above the
  threshold.
- Power must stay safe for the full stability delay before any wake. A lost or
  expired reading, a restart, a storage failure, a gap between checks longer
  than `nut.max_reading_age`, or time moving backwards starts the delay again.
- Only devices that answered ping on mains just before the outage are woken. A
  normal startup with nothing pending wakes nothing.
- Devices are watched until waking is allowed. One that answers ping then is
  done and is never woken, even if it shuts down later.
- Only an offline ping result triggers a wake; an unknown result never does.
- Device timeouts count down only while waking is allowed.
- An attempt is saved before its packet is sent; if saving fails, nothing is
  sent. A packet is only sent while its UPS reading is still fresh.
- Recovery progress survives restarts and repeated outages. Damaged or
  unfamiliar database files are refused and left untouched.
- History is best effort: failing to save it never blocks a wake.
- The API keeps its Host and Origin checks whether or not a reverse proxy is in
  front of it.

## Conventions

- **Names say completely what something is or does.** Distinguish
  configuration, saved state, observations, and network results.
- **No comments** unless they explain non-obvious reasoning.
- **Keep changes focused.** No drive-by refactors or reformatting.
- **Tests use Given/When/Then.** Go tests are named
  `TestGiven<Situation>When<Action>Then<Outcome>` with `// Given`, `// When`
  and `// Then` comments. Dashboard tests nest `describe('Given …')` →
  `describe('When …')` → `it('Then …')`, each `it` checking one outcome.
- **Tests never touch a real UPS or real devices.** Use simulated readings,
  fake ping runners, loopback listeners, temporary databases, and documentation
  addresses such as `192.0.2.10`. Assert what was woken, saved, or refused, not
  implementation details.
- **Docs follow behaviour.** User-visible changes update `README.md` and
  `CHANGELOG.md`; new settings update `config.example.yaml`; recovery changes
  update the safety rules above. Keep examples generic.

## Pull requests and releases

- Fill in `.github/pull_request_template.md`. Dependency changes also need
  `make security-check`. Third-party notices are generated at build time;
  only copied component source and packages used just by the stylesheet need
  a notice added to `web/THIRD_PARTY_LICENSES`. Changes to `deploy/`, the
  `Dockerfile`, or workflows also need `docker build` and `make release`.
- GitHub Actions are pinned to version tags, and each check is a separate job.
- Pull requests merge into `main` only when up to date and every Verify job
  passes. Nothing is published on merge: a maintainer updates `VERSION` and
  `CHANGELOG.md`, merges, and runs **Release** in GitHub Actions.

## Boundaries

**Ask the maintainer first:** publishing, pushing, tagging, or running Release;
changing GitHub or package settings; starting or restarting a live deployment;
changing local deployment configuration or deleting recovery data; changing
configuration keys, database compatibility, or when devices are woken beyond
what the change was for; adding dependencies.

**Never:** commit or expose credentials or private deployment data (`.env`,
`config.yaml`, `compose.override.yaml`, database files); use a live UPS or real
devices as test targets; overwrite unrelated changes in the working tree;
hand-edit generated code, dependencies, or build outputs.
