# API Stability

This document defines what "stable" means for Vane's public Go packages, and how each
stability category is marked.

**Status: pre-v1.0.0.** The guarantees below take effect at `v1.0.0`. Until then, Vane
follows standard pre-1.0 semver: any API may change in a minor or patch release.

## Public packages

```text
github.com/filipejohansson/vane/core
github.com/filipejohansson/vane/core/router
github.com/filipejohansson/vane/core/signal
github.com/filipejohansson/vane/core/domattrs
```

Anything exported from these packages is part of the public API. Anything under
`internal/` is not.

## Categories

### Stable

The default for anything exported from a public package above. No marker needed: absence of
an Experimental or Deprecated signal (below) means Stable.

Stable APIs won't have a breaking signature or behavior change outside a major version.

### Deprecated

Marked with a standard Go doc comment:

```go
// Deprecated: use NewFoo instead. Bar will be removed in the next major version.
func Bar() {}
```

A deprecated symbol stays available for at least one full minor version before removal, and
is only ever removed in a major release.

### Experimental

- **A single symbol inside an otherwise-stable package**: its doc comment starts with
  `EXPERIMENTAL:`.
- **A whole surface still being proven**: a separate import path outside the public packages
  above, promoted into one of them once stable.

Experimental APIs can change or be removed in a minor release.

## Enforcement

A committed snapshot of the four packages' exported symbols is diffed in CI against the
current source on every PR. Adding, removing, or renaming an exported symbol — or changing an
`EXPERIMENTAL:`/`Deprecated:` marker — requires updating the snapshot in the same PR.
