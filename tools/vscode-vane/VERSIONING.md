# Versioning

This extension uses [Semantic Versioning](https://semver.org/) (`MAJOR.MINOR.PATCH`).

**Status: pre-v1.0.0**, currently `0.1.0`. The guarantees below take effect at the extension's own
`v1.0.0`. Until then, any release may include breaking changes, in a minor or even a patch version.

## Pre-1.0.0 (current)

- **Patch** (`0.x.Y`): bug fixes, docs, packaging/tooling changes.
- **Minor** (`0.X.0`): new features, and any breaking change.

## Post-1.0.0

- **Patch**: bug fixes only. No change to commands, settings, or their behavior.
- **Minor**: backward-compatible additions — new commands, new settings, new diagnostics/
  features. Existing commands, settings, and keybindings keep working unchanged.
- **Major**: breaking changes — a command is removed or renamed, a setting's name or meaning
  changes, or the minimum required VS Code (`engines.vscode`) or `vane` CLI version is raised in
  a way that drops support for a previously supported version.

## What becomes stable at v1.0.0

1. **Commands** — the IDs and behavior of `vane.restartLanguageServer`, `vane.openGeneratedGoFile`,
   and any other registered command.
2. **Settings** — any configuration property this extension contributes, its name, and its
   meaning/default.
3. **Minimum supported versions** — the `engines.vscode` range in `package.json`, and the minimum
   `vane` CLI version this extension's `vane lsp` integration requires.

A change to any of these outside a major version is a bug, not an intentional release.
