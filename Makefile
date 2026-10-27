VERSION ?= $(shell cat VERSION)
GOFUMPT_VERSION := v0.12.0
STATICCHECK_VERSION := v0.8.2
ACTIONLINT_VERSION := v1.7.12
GOVULNCHECK_VERSION := v1.9.0
SQLC_VERSION := v1.31.1
GO_PACKAGES := ./cmd/... ./internal/...

.PHONY: build licenses test check lint go-lint frontend frontend-lint frontend-test generate generate-check format format-check go-format-check workflow-check shell-check release security-check
build: licenses
	go build -trimpath -ldflags="-X main.version=$(VERSION)" -o bin/evigo ./cmd/evigo
test:
	go test -race $(GO_PACKAGES)
format:
	go run mvdan.cc/gofumpt@$(GOFUMPT_VERSION) -w cmd internal
	cd web && npm run lint:fix && npm run format
go-format-check:
	@files="$$(go run mvdan.cc/gofumpt@$(GOFUMPT_VERSION) -l cmd internal)" || exit 1; \
	if [ -n "$$files" ]; then \
		printf '%s\n' "$$files"; \
		printf '%s\n' 'Go formatting differs; run make format.' >&2; \
		exit 1; \
	fi
format-check: go-format-check
	cd web && npm run format:check
lint: go-lint frontend-lint
generate:
	go run github.com/sqlc-dev/sqlc/cmd/sqlc@$(SQLC_VERSION) generate
generate-check:
	go run github.com/sqlc-dev/sqlc/cmd/sqlc@$(SQLC_VERSION) diff
go-lint: go-format-check generate-check
	go vet $(GO_PACKAGES)
	go run honnef.co/go/tools/cmd/staticcheck@$(STATICCHECK_VERSION) $(GO_PACKAGES)
shell-check:
	shellcheck -s sh deploy/install.sh scripts/third-party-licenses.sh
security-check:
	go run golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION) $(GO_PACKAGES)
	cd web && npm audit --audit-level=high
workflow-check:
	go run github.com/rhysd/actionlint/cmd/actionlint@$(ACTIONLINT_VERSION)
check: lint shell-check workflow-check
	go test -race $(GO_PACKAGES)
	cd web && npm test
release: licenses
	mkdir -p dist
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w -X main.version=$(VERSION)" -o dist/evigo-linux-amd64 ./cmd/evigo
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags="-s -w -X main.version=$(VERSION)" -o dist/evigo-linux-arm64 ./cmd/evigo
	install -m 0644 LICENSE config.example.yaml deploy/evigo.service dist/
	install -m 0644 internal/api/static/licenses.txt dist/THIRD_PARTY_LICENSES
	install -m 0755 deploy/install.sh dist/
	COPYFILE_DISABLE=1 tar --no-xattrs -czf dist/evigo-linux-amd64.tar.gz -C dist evigo-linux-amd64 LICENSE THIRD_PARTY_LICENSES config.example.yaml evigo.service
	COPYFILE_DISABLE=1 tar --no-xattrs -czf dist/evigo-linux-arm64.tar.gz -C dist evigo-linux-arm64 LICENSE THIRD_PARTY_LICENSES config.example.yaml evigo.service
	cd dist && if command -v sha256sum >/dev/null 2>&1; then sha256sum evigo-linux-amd64.tar.gz evigo-linux-arm64.tar.gz install.sh; else shasum -a 256 evigo-linux-amd64.tar.gz evigo-linux-arm64.tar.gz install.sh; fi > SHA256SUMS
licenses: frontend
	sh scripts/third-party-licenses.sh > internal/api/static/licenses.txt
frontend:
	cd web && npm ci && npm run build
frontend-lint:
	cd web && npm run format:check && npm run lint && npm run typecheck
frontend-test:
	cd web && npm ci && npm test
