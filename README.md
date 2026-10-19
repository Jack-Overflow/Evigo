# Evigo

Evigo turns your machines back on after a power cut.

When the power goes out, your UPS keeps things running for a while and then your
existing shutdown setup powers machines off safely. Evigo watches
your UPS, remembers which machines were running before the outage, and wakes
them with Wake-on-LAN once the power is back and safe.

Evigo only ever turns machines **on**. It never shuts anything down and never
sends commands to your UPS, so keep your existing shutdown setup as it is.

## How it works

1. **Before an outage:** Evigo keeps an eye on your UPS and checks which of your
   machines are online.
2. **When the power goes out:** it notes which machines were running just before.
   Machines that were already off are left alone.
3. **When the power comes back:** it waits until power has been steady for a short
   while and the UPS battery has recharged enough to cover another outage.
4. **Then it wakes your machines,** retrying every so often until each one comes
   back online, or gives up after a while if one never does.

If Evigo itself restarts in the middle of this, for example because the machine
it runs on also lost power, it picks up where it left off.

A missing or expired UPS reading restarts the full stability delay. So does a
pause between checks longer than `nut.max_reading_age`. Unobserved time does not
use up a machine's recovery timeout or shutdown grace. Each wake packet must be
sent before its UPS reading expires.

Recording an attempt does not mean its packet was sent. Failed or skipped sends
cannot bypass shutdown grace, and another outage starts grace again. After a
restart, any remaining grace is honoured until a new packet is sent or grace
expires. A saved attempt timestamp ahead of the current clock does not prevent
retrying once the full stability delay has passed.

## What you need

- A UPS monitored by [Network UPS Tools (NUT)](https://networkupstools.org/), with
  its server reachable from where Evigo runs.
- Machines with **Wake-on-LAN enabled** in their BIOS/UEFI and network settings.
- An always-on Linux machine on the **same network** as the machines you want to
  wake, to run Evigo. Docker or a systemd-based Linux install both work.
- Each machine's IP address (or hostname) and MAC address.

## Quick start with Docker

1. Download [`compose.yaml`](compose.yaml) and
   [`config.example.yaml`](config.example.yaml) into a new folder, and rename the
   example to `config.yaml`.
2. Edit `config.yaml` with your UPS and machines (see [Configuration](#configuration)).
3. Check the configuration:

   ```sh
   docker compose run --rm evigo validate --config /etc/evigo/config.yaml
   ```

4. Start Evigo and watch the logs:

   ```sh
   docker compose up -d
   docker compose logs -f evigo
   ```

You should see `Evigo started` followed by your UPS status.

Docker must run on a Linux host.

## Installing on Linux without Docker

```sh
curl -fsSL https://github.com/Jack-Overflow/Evigo/releases/latest/download/install.sh -o install.sh
sudo sh ./install.sh
```

The installer downloads and verifies the latest release, creates an `evigo`
system user, and installs a systemd service. It doesn't start anything. Then:

```sh
sudoedit /etc/evigo/config.yaml
sudo -u evigo evigo validate --config /etc/evigo/config.yaml
sudo systemctl enable --now evigo
journalctl -u evigo -f
```

The installer needs systemd, `curl` or `wget`, `tar`, `awk`, `sha256sum` and the
iputils version of `ping`, which most distributions include. It tells you if
anything is missing.

## Configuration

Everything lives in one YAML file. The [example](config.example.yaml) lists every
setting with its default, so you only need to fill in your own details:

```yaml
nut:
  host: 192.168.1.5     # your NUT server
  ups: myups            # the UPS name as NUT knows it
devices:
  - id: nas
    name: NAS
    host: 192.168.1.20
    mac: 'aa:bb:cc:dd:ee:01'
  - id: server
    name: Home server
    host: 192.168.1.21
    mac: 'aa:bb:cc:dd:ee:02'
```

Restart Evigo after changing the file. Run `evigo validate` first: for a mistyped
or invalid setting, it tells you which one and on which line.

### Settings you might want to change

| Setting                          | Default | What it does                                                            |
| -------------------------------- | ------- | ----------------------------------------------------------------------- |
| `recovery.min_battery_percent`   | `25`    | How charged the UPS battery must be before machines are woken.          |
| `recovery.stable_power_delay`    | `30s`   | How long power must stay on before waking anything.                     |
| `recovery.shutdown_grace_period` | `2m`    | Set this longer than your slowest machine takes to shut down.           |
| `recovery.retry_interval`        | `30s`   | How often to resend the wake signal to a machine that hasn't come back. |
| `recovery.device_timeout`        | `10m`   | How long to keep trying before giving up on a machine.                  |

### NUT login

If your NUT server needs a login, add a username and password. You can keep them
out of the file by using environment variables (with Docker, put them in a `.env`
file next to `compose.yaml`) or a password file:

```yaml
nut:
  username: monitor
  password: "${NUT_PASSWORD}"
  # or: password_file: /etc/evigo/nut-password
```

NUT sends passwords unencrypted, so only use it on a network you trust.

### Machines on another network

Wake-on-LAN signals don't normally cross between networks or VLANs. Either run
Evigo on the same network as the machines, or set a directed broadcast address
for the machine (your router must allow it):

```yaml
devices:
  - id: nas
    name: NAS
    host: 192.168.20.20
    mac: 'aa:bb:cc:dd:ee:01'
    wol:
      broadcast: 192.168.20.255
```

## Status API

Evigo can serve its current status and recent history over HTTP. It's off until
you choose an address for it:

```yaml
web:
  listen: 127.0.0.1:8080
```

There is **no login**. Anyone who can reach it can see your UPS and machines, so
keep it on `127.0.0.1` or put it behind a reverse proxy that adds a login and
HTTPS. If you reach it by a hostname, list that name so Evigo accepts it:

```yaml
web:
  listen: 127.0.0.1:8080
  allowed_hosts: [evigo.example.lan]
```

| Request                               | Returns                                                                                                    |
| ------------------------------------- | ---------------------------------------------------------------------------------------------------------- |
| `GET /api/status`                     | UPS status, each machine's reachability, recovery progress, and why waking is blocked right now, if it is. |
| `GET /api/history?limit=50&before=ID` | Recent events, newest first. Pass `next_before` as `before` for older ones.                                |
| `GET /health`                         | `200` while Evigo is checking your UPS and machines normally; useful for uptime monitors.                  |

Evigo keeps the newest 10,000 history events and deletes older ones as new ones
arrive. When you upgrade from 0.1.0, the database is upgraded automatically and
any recovery in progress is kept.

## Troubleshooting

**A machine wasn't woken.** Check that:
- it was online when the power went out (machines that were already off are left
  off on purpose),
- Wake-on-LAN works for it, for example by testing with another Wake-on-LAN tool,
- the battery reached `min_battery_percent`; the logs show the UPS status and
  battery level.

**Every machine shows as unknown.** Evigo checks machines with `ping`. In Docker,
the host must allow the container's group (`10001`) to ping:

```sh
sysctl net.ipv4.ping_group_range
```

If `10001` isn't inside that range, widen it in `/etc/sysctl.d/`. For a native
install, do the same for the `evigo` group the installer printed. Machines Evigo
can't check are never woken, so this is worth fixing first.

**A slow machine was missed.** If it was still shutting down when the power came
back, increase `shutdown_grace_period` so it's longer than the machine's
shutdown.

**Evigo won't start because the web address is in use.** Pick a different
`web.listen` port, or leave it empty to turn it off.

**Evigo won't start because of its database.** Evigo refuses to touch a database
file it doesn't recognise, including leaving its journal mode unchanged. Stop it,
move the file aside, and start again.

## Upgrading

- **Docker:** `docker compose pull && docker compose up -d`
- **Linux:** stop the service, run the installer again, then start the service.
  Your configuration and saved progress are kept.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) to get started and [AGENTS.md](AGENTS.md)
for architecture, safety rules and development commands.

## Security

See [SECURITY.md](SECURITY.md) for reporting vulnerabilities privately.

## License

Apache-2.0. See [LICENSE](LICENSE) and [third-party notices](THIRD_PARTY_LICENSES).
