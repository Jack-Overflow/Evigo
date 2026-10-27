#!/bin/sh
set -eu

notice() {
	printf '\n## %s\n\n' "$1"
	for file in "$2"/LICENSE* "$2"/NOTICE* "$2"/PATENTS; do
		if [ -f "$file" ]; then
			cat "$file"
		fi
	done
}

printf '# Go modules\n'
notice "Go standard library $(go env GOVERSION)" "$(go env GOROOT)"
for architecture in amd64 arm64; do
	CGO_ENABLED=0 GOOS=linux GOARCH=$architecture \
		go list -deps -f '{{with .Module}}{{if not .Main}}{{.Path}} {{.Version}} {{.Dir}}{{end}}{{end}}' ./cmd/evigo
done | sort -u | while read -r path version directory; do
	notice "$path $version" "$directory"
done

printf '\n'
cat internal/api/static/.vite/license.md
