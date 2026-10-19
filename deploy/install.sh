#!/bin/sh
set -eu

fail() { printf 'Evigo installer: %s\n' "$*" >&2; exit 1; }
usage() {
  printf '%s\n' 'Usage: install.sh [--version VERSION]' \
    'Downloads and verifies the latest release from Jack-Overflow/Evigo; preserves config and state.' \
    'Requires systemd and iputils ping. Does not install OS packages or start services.'
}

repo=Jack-Overflow/Evigo
version=latest
while [ "$#" -gt 0 ]; do
  case "$1" in
    --version) [ "$#" -ge 2 ] || fail '--version needs a value'; version=${2#v}; shift 2 ;;
    --help|-h) usage; exit 0 ;;
    *) fail "unknown option: $1" ;;
  esac
done
[ "$(uname -s)" = Linux ] || fail 'this installer targets Linux'
[ "$(id -u)" = 0 ] || fail 'run with sudo or as root'
command -v systemctl >/dev/null 2>&1 || fail 'the native installer requires systemd; otherwise install the standalone binary manually'
case "$(uname -m)" in
  x86_64|amd64) arch=amd64 ;;
  aarch64|arm64) arch=arm64 ;;
  *) fail 'release binaries support x86_64 and aarch64; build from source for another architecture' ;;
esac
if [ "$version" != latest ]; then
  printf '%s\n' "$version" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+([.-][A-Za-z0-9.-]+)?$' || fail 'invalid release version'
fi
command -v tar >/dev/null 2>&1 || fail 'tar is required'
command -v awk >/dev/null 2>&1 || fail 'awk is required'
command -v sha256sum >/dev/null 2>&1 || fail 'sha256sum is required'
if command -v curl >/dev/null 2>&1; then
  download() { curl --fail --silent --show-error --location --proto '=https' --proto-redir '=https' --connect-timeout 10 --max-time 300 --retry 2 --output "$2" "$1"; }
elif command -v wget >/dev/null 2>&1; then
  download() { wget -q -T 30 -t 3 -O "$2" "$1"; }
else
  fail 'curl or wget is required'
fi

helpers_ready() {
  command -v ping >/dev/null 2>&1 || return 1
  ping -V 2>&1 | grep -qi iputils || return 1
}
helpers_ready || fail 'install iputils ping using your distribution tools, then rerun (BusyBox ping is not supported)'

scratch=$(mktemp -d)
temporary=
trap 'rm -rf "$scratch"; if [ -n "$temporary" ]; then rm -f "$temporary"; fi' EXIT
trap 'exit 1' HUP INT TERM
archive="evigo-linux-$arch.tar.gz"
if [ "$version" = latest ]; then
  base="https://github.com/$repo/releases/latest/download"
else
  base="https://github.com/$repo/releases/download/v$version"
fi
download "$base/$archive" "$scratch/$archive"
download "$base/SHA256SUMS" "$scratch/SHA256SUMS"
# Verify just the selected archive, before extracting or executing anything.
awk -v name="$archive" '$2 == name { print; count++ } END { if (count != 1) exit 1 }' "$scratch/SHA256SUMS" > "$scratch/selected.sha256" || fail 'release checksum is missing or ambiguous'
(cd "$scratch" && sha256sum -c selected.sha256) || fail 'release checksum verification failed'
package="$scratch/package"
mkdir "$package"
# Extract only expected release members into the private temporary directory.
tar -xzf "$scratch/$archive" -C "$package" "evigo-linux-$arch" config.example.yaml evigo.service LICENSE THIRD_PARTY_LICENSES
binary="$package/evigo-linux-$arch"
for file in "evigo-linux-$arch" config.example.yaml evigo.service LICENSE THIRD_PARTY_LICENSES; do
  if [ ! -f "$package/$file" ] || [ -L "$package/$file" ]; then
    fail "release archive is missing a regular $file"
  fi
done
"$binary" version >/dev/null || fail 'release binary cannot run on this host'

if ! id evigo >/dev/null 2>&1; then
  command -v useradd >/dev/null 2>&1 || fail 'create an evigo system account with its own group, then rerun'
  if ! grep -q '^evigo:' /etc/group; then
    command -v groupadd >/dev/null 2>&1 || fail 'create the evigo system group, then rerun'
    groupadd -r evigo
  fi
  useradd -r -g evigo -M -d /var/lib/evigo -s /sbin/nologin evigo
fi
group=$(id -gn evigo)
[ "$(id -u evigo)" != 0 ] || fail 'the evigo account must be unprivileged'
[ "$group" = evigo ] || fail 'the existing evigo account must have evigo as its primary group'
install -d -m 0755 /usr/local/bin
install -d -m 0750 -o root -g "$group" /etc/evigo
install -d -m 0700 -o evigo -g "$group" /var/lib/evigo
install -d -m 0755 /usr/local/share/licenses/evigo
# Rename a completed replacement so an existing executable is never truncated.
temporary=$(mktemp /usr/local/bin/.evigo.XXXXXX)
install -m 0755 "$binary" "$temporary"
mv -f "$temporary" /usr/local/bin/evigo
if [ ! -e /etc/evigo/config.yaml ]; then
  install -m 0640 -o root -g "$group" "$package/config.example.yaml" /etc/evigo/config.yaml
fi
for file in LICENSE THIRD_PARTY_LICENSES; do
  install -m 0644 "$package/$file" "/usr/local/share/licenses/evigo/$file"
done
install -d -m 0755 /etc/systemd/system
install -m 0644 "$package/evigo.service" /etc/systemd/system/evigo.service
if [ -d /run/systemd/system ]; then systemctl daemon-reload; fi
printf '\n'
printf '%s\n' 'Evigo installed. Existing configuration and state were preserved.' \
  '1. Edit /etc/evigo/config.yaml for your UPS and devices.' \
  '2. Validate: sudo /usr/local/bin/evigo validate --config /etc/evigo/config.yaml' \
  "3. Check unprivileged ping: the evigo group (GID $(id -g evigo)) must be allowed by net.ipv4.ping_group_range." \
  '4. Start when ready: sudo systemctl enable --now evigo' \
  'No service was started or restarted. Installer does not modify network/sysctl settings.'
