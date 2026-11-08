# Changelog

## 0.3.1

- A single lost ping no longer marks a device offline.

## 0.3.0

- Optional alerts by email, Discord, ntfy or webhook when the UPS goes on
  battery, mains power returns, a woken device comes back, or Evigo gives up on
  a device.
- `evigo test-alerts` sends a test alert to each configured provider and
  reports which ones worked.

## 0.2.2

- Tooltips open when tapped on phones and tablets.
- Dependency updates.

## 0.2.1

- The battery minimum is explained in a tooltip on its marker instead of a
  caption.
- History events sit above their time on phones instead of wrapping unevenly.

## 0.2.0

- Optional read-only dashboard and status API at the `web.listen` address.

## 0.1.0

- Wakes devices with Wake-on-LAN after a UPS outage once power is safe and
  stable, retrying until each comes back or times out.
- Recovery progress is saved and resumed after a restart.
- Docker image, Linux amd64/arm64 builds, installer and systemd unit.
