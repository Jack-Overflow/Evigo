# Changelog

## 0.1.0

Initial release.

- One loop: read the UPS over NUT, ping devices, update recovery, save, wake.
- Devices online on mains just before the UPS goes on battery are woken once mains
  power has been safe and stable and the battery is above the threshold. Devices
  already off stay off.
- Retries on an interval, a per-device timeout that only counts while waking is
  allowed, and a shutdown grace period so a machine still shutting down is not
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
