# Security Policy

## Supported versions

Vane is pre-1.0.0. Only the latest published release receives security
fixes; there are no parallel maintenance branches yet.

| Version           | Supported          |
| ------------------ | ------------------ |
| Latest release      | :white_check_mark: |
| Older pre-1.0.0 tags | :x:                 |

Once `v1.0.0` ships, this table will be updated with a concrete support
window covering more than just the latest release.

## Reporting a vulnerability

**Do not open a public GitHub issue for security vulnerabilities.**

Report it privately using
[GitHub's private vulnerability reporting](https://github.com/FilipeJohansson/vane/security/advisories/new)
for this repository (visible under the repo's **Security** tab → **Report a
vulnerability**). This opens a private advisory visible only to you and the
maintainer until a fix is ready.

Include, where relevant:

- The affected version/commit.
- A minimal reproduction (`.vane`/`.go` snippet, or a link to a repo).
- The impact you believe it has (e.g. XSS via unescaped raw HTML, DOM
  injection, unsafe property handling, URL/routing-based attacks).

## What to expect

Vane is maintained by a small team, so response times aren't guaranteed by
an SLA, but every report will get an acknowledgment. Once a fix is
available, it will be released and, where appropriate, disclosed via a
GitHub Security Advisory crediting the reporter (unless anonymity is
requested).

## Scope

This policy covers the Vane compiler, runtime (`core`, `core/router`), and
CLI in this repository. It does not cover applications built with Vane —
report vulnerabilities in those to their own maintainers.
