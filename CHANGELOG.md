# Changelog

## 0.2.0

- Optional read-only dashboard at the `web.listen` address: power and battery,
  each device's reachability and recovery progress, and history. It marks the
  last known state as outdated when Evigo stops answering or checking.
- Optional read-only status API, turned on with `web.listen`: `GET /api/status`
  shows UPS status, each device's reachability, recovery progress, and why
  waking is blocked; `GET /health` reports whether Evigo is checking normally,
  as `monitor`: `running`, `stalled` or `starting`.
  Host and Origin checks stop DNS rebinding and cross-site requests.
- History of UPS status changes, reachability changes, outages, wakes and
  results at `GET /api/history`, keeping the newest 10,000 events.

## 0.1.0

Initial release.

- One loop: read the UPS over NUT, ping devices, update recovery, save, wake.
- Devices online on mains just before the UPS goes on battery are woken once mains
  power has been safe and stable and the battery is above the threshold. Devices
  already off stay off.
- Retries on an interval, a per-device timeout that only counts while waking is
  allowed, and a shutdown grace period so a device still shutting down is not
  counted as recovered.
- Pending devices saved in SQLite before any packet is sent and resumed after a
  restart.
- Interrupted or stale observations restart stability without spending recovery
  timers; every wake has a deadline bounded by the UPS reading's expiry.
- Failed or skipped packets cannot bypass shutdown grace, repeated outages renew
  grace, and future saved timestamps cannot suppress retries after restart.
- Incompatible SQLite databases are refused before changing their journal mode.
- Strict YAML configuration, environment and file credentials, and JSON logs
  containing only recognised UPS status flags or UNKNOWN.
- `run`, `validate` and `version` commands.
- Docker and static native Linux amd64/arm64 builds, an installer and a systemd unit.
- CI verifies with the latest Go 1.26 patch release, including security fixes.
- Public repository module path and contributor and security reporting guides.
