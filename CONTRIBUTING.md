# Contributing to Evigo

Read [AGENTS.md](AGENTS.md) before making changes. It covers the architecture,
safety rules, code style, tests and release process for both human contributors
and coding tools.

Use the latest Go 1.26 patch release or newer, Node 24 (LTS) with npm,
ShellCheck, and Docker for container checks. Run `make check` before opening a pull request. Dependency
changes also need `make security-check`. Third-party notices are generated at
build time; only copied component source and packages used just by the
stylesheet need a notice added to `web/THIRD_PARTY_LICENSES`.

Keep pull requests focused, describe the problem and resulting behaviour, and
include the checks you ran. Add regression tests for behaviour changes and
update the user documentation when needed. Tests must use fakes and loopback
listeners, never a live UPS or real devices.

Use GitHub issues for bugs and feature requests. Remove credentials, private
hostnames, device addresses and deployment details from anything you share.
For vulnerabilities, follow [SECURITY.md](SECURITY.md).
