# Contributing to Evigo

Read [AGENTS.md](AGENTS.md) before making changes. It covers the architecture,
safety rules, code style, tests and release process for both human contributors
and coding tools.

Use the latest Go 1.26 patch release or newer, Node 24 (LTS) with npm,
ShellCheck, and Docker for container checks. Run `make check` before opening a pull request. Dependency
changes also need `make security-check` and updated `THIRD_PARTY_LICENSES` notices.

Keep pull requests focused, describe the problem and resulting behaviour, and
include the checks you ran. Add regression tests for behaviour changes and
update the user documentation when needed. Tests must use fakes and loopback
listeners, never a live UPS or real machines.

Use GitHub issues for bugs and feature requests. Remove credentials, private
hostnames, device addresses and deployment details from anything you share.
For vulnerabilities, follow [SECURITY.md](SECURITY.md).
