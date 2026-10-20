# Changelog

## 0.2.0

- Optional read-only dashboard and status API at the `web.listen` address.

## 0.1.0

- Wakes devices with Wake-on-LAN after a UPS outage once power is safe and
  stable, retrying until each comes back or times out.
- Recovery progress is saved and resumed after a restart.
- Docker image, Linux amd64/arm64 builds, installer and systemd unit.
