# Configuration

Evigo reads one YAML file, passed with `--config`. Docker and the systemd
service use `/etc/evigo/config.yaml`. Every setting except `nut.host`, `nut.ups`
and `devices` has a default. Unknown settings are rejected, so `evigo validate`
catches typos and tells you the line.

Durations use Go's format, such as `30s`, `2m` or `1h`, and must be positive and
at most `168h`.

## `nut`: the UPS

| Setting           | Default  | What it does                                                                                    |
| ----------------- | -------- | ----------------------------------------------------------------------------------------------- |
| `host`            | required | Address of your NUT server.                                                                     |
| `ups`             | required | The UPS name as NUT knows it.                                                                   |
| `port`            | `3493`   | NUT server port.                                                                                |
| `username`        | none     | NUT login. Set both `username` and a password, or neither.                                      |
| `password`        | none     | NUT password. `"${NAME}"` reads it from the environment variable `NAME`.                        |
| `password_file`   | none     | Read the password from this file instead of `password`.                                         |
| `connect_timeout` | `5s`     | How long to wait to connect to NUT.                                                             |
| `read_timeout`    | `5s`     | How long to wait for each reading.                                                              |
| `max_reading_age` | `30s`    | How old a reading may be and still count. Must be at least `poll.interval` plus `read_timeout`. |

With Docker, set `NUT_USERNAME` and `NUT_PASSWORD` in a `.env` file next to
`compose.yaml`, and use `"${NUT_USERNAME}"` and `"${NUT_PASSWORD}"` in the
configuration. Evigo connects to NUT without encryption, so only use a
login on a network you trust.

## `poll`: how often Evigo checks

| Setting               | Default | What it does                          |
| --------------------- | ------- | ------------------------------------- |
| `interval`            | `10s`   | Time between checks on mains power.   |
| `on_battery_interval` | `2s`    | Time between checks while on battery. |

## `recovery`: when devices are woken

| Setting               | Default | What it does                                                                                 |
| --------------------- | ------- | -------------------------------------------------------------------------------------------- |
| `min_battery_percent` | `25`    | Battery charge (0–100) needed before waking anything.                                        |
| `stable_power_delay`  | `30s`   | How long power must stay safe before waking. Make it longer than a shutdown takes to finish. |
| `retry_interval`      | `30s`   | How often to resend a wake to a device that hasn't come back.                                |
| `device_timeout`      | `10m`   | How long to keep waking a device before giving up. Only counts while waking is allowed.      |

## `probe`: checking devices

| Setting   | Default | What it does                       |
| --------- | ------- | ---------------------------------- |
| `timeout` | `2s`    | How long to wait for a ping reply. |

## `wol`: Wake-on-LAN defaults

| Setting     | Default           | What it does                           |
| ----------- | ----------------- | -------------------------------------- |
| `broadcast` | `255.255.255.255` | IPv4 address wake packets are sent to. |
| `port`      | `9`               | UDP port wake packets are sent to.     |

## `devices`: what to wake

At least one device is required.

| Setting         | Default         | What it does                                                                      |
| --------------- | --------------- | --------------------------------------------------------------------------------- |
| `id`            | required        | Unique ID: letters, digits, `.`, `-` or `_`. Keeps saved progress across renames. |
| `name`          | required        | Name shown in the dashboard and logs.                                             |
| `host`          | required        | IP address or hostname to ping.                                                   |
| `mac`           | required        | The device's MAC address (a unicast, 6-byte address).                             |
| `wol.broadcast` | `wol.broadcast` | This device's broadcast address, for devices on another network.                  |
| `wol.port`      | `wol.port`      | This device's wake port.                                                          |

## `web`: dashboard and API

| Setting         | Default | What it does                                                                                          |
| --------------- | ------- | ----------------------------------------------------------------------------------------------------- |
| `listen`        | off     | IP address and port to serve the dashboard on, such as `127.0.0.1:8080`. Empty turns it off.          |
| `allowed_hosts` | none    | Hostnames, without a port, the dashboard may be reached by. IP addresses and `localhost` always work. |

## `log` and `state`

| Setting      | Default      | What it does                                                                      |
| ------------ | ------------ | --------------------------------------------------------------------------------- |
| `log.level`  | `info`       | `debug`, `info`, `warn` or `error`.                                               |
| `state.path` | `./evigo.db` | Where recovery progress and history are saved. Docker and the installer set this. |
