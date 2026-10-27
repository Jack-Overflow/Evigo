# Evigo

Evigo turns your devices back on after a power cut. It watches your UPS through
[Network UPS Tools (NUT)](https://networkupstools.org/), remembers which devices
were on when the power went out, and wakes them with Wake-on-LAN once power is
back and safe.

It only ever turns devices **on**. Keep your existing shutdown setup as it is.

![The Evigo dashboard, light theme on the left and dark on the right](docs/dashboard.png)

## How it works

1. When the UPS goes on battery, Evigo notes which devices are online.
2. When mains returns, it waits until power has been stable for a while and the
   battery has recharged to your chosen minimum.
3. It wakes the devices that are off, retrying until each answers or it gives
   up.

If Evigo restarts part-way through, it carries on where it left off.

## What you need

- A UPS monitored by NUT, reachable from where Evigo runs.
- Devices with Wake-on-LAN enabled. If your UPS cuts its outlets at the end of
  an outage, also enable waking after a full power loss in the BIOS.
- An always-on Linux host on the same network as the devices (or see
  [devices on another network](#devices-on-another-network)).

## Install

### Docker

Docker must run on a Linux host, because Evigo needs host networking to reach
your devices.

1. Download [`compose.yaml`](compose.yaml) and
   [`config.example.yaml`](config.example.yaml), and rename the example to
   `config.yaml`.
2. Fill in your UPS and devices (see [Configuration](#configuration)).
3. Check it, then start Evigo:

   ```sh
   docker compose run --rm evigo validate --config /etc/evigo/config.yaml
   docker compose up -d
   docker compose logs -f evigo
   ```

### Linux with systemd

```sh
curl -fsSL https://github.com/Jack-Overflow/Evigo/releases/latest/download/install.sh -o install.sh
sudo sh ./install.sh
sudoedit /etc/evigo/config.yaml
sudo -u evigo evigo validate --config /etc/evigo/config.yaml
sudo systemctl enable --now evigo
```

The installer verifies the release, creates an `evigo` user and installs the
service without starting it. It needs systemd and the iputils `ping`.

## Configuration

Everything is in one YAML file. Fill in your UPS and devices:

```yaml
nut:
  host: 192.0.2.5 # your NUT server
  ups: myups # the UPS name in NUT
devices:
  - id: nas
    name: NAS
    host: 192.0.2.20
    mac: "02:00:00:00:00:01"
```

See [all settings](docs/configuration.md) for every option and its default.

### Devices on another network

Wake-on-LAN doesn't normally cross networks or VLANs. Set the device's subnet
broadcast address instead (your router must allow directed broadcasts):

```yaml
devices:
  - id: nas
    name: NAS
    host: 198.51.100.20
    mac: "02:00:00:00:00:01"
    wol:
      broadcast: 198.51.100.255
```

## Dashboard

Set `web.listen` to turn on the read-only dashboard and its API:

```yaml
web:
  listen: 127.0.0.1:8080
```

It shows power, battery, each device and recent history, and says so when
Evigo stops answering. There is **no login**: keep it on `127.0.0.1` or behind
a reverse proxy that adds one. If you reach it by hostname, add the name to
`web.allowed_hosts` (without a port).

The API serves `GET /api/status`, `GET /api/history` and `GET /health`, which
returns `200` while Evigo is checking normally, for uptime monitors.

## Troubleshooting

**A device wasn't woken.** It must have been on when the power went out and off
once power was stable, and Wake-on-LAN must work for it, including after a full
power loss. The battery must also have reached `min_battery_percent`.

**Every device shows as unknown.** Evigo needs to run `ping` without root.
Check that its group (`10001` in Docker, or the `evigo` group) is inside
`sysctl net.ipv4.ping_group_range`, and widen the range in `/etc/sysctl.d/` if
not.

**A slow device was missed.** Increase `stable_power_delay` so it's longer than
the device takes to shut down.

## Upgrading

- **Docker:** `docker compose pull && docker compose up -d`
- **Linux:** run the installer again, then restart the service. Your
  configuration is kept.

## Contributing and security

See [CONTRIBUTING.md](CONTRIBUTING.md), [AGENTS.md](AGENTS.md) and
[SECURITY.md](SECURITY.md).

## License

Apache-2.0. See [LICENSE](LICENSE). Third-party notices ship as
`THIRD_PARTY_LICENSES` in release archives and the container image.
