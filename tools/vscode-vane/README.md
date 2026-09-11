# Vane for VS Code

Syntax highlighting and a real language server for `.vane` files — [Vane](https://github.com/filipejohansson/vane)'s Go/WebAssembly frontend framework with JSX-like syntax.

## Features

- **Syntax highlighting** for `.vane` files: Go keywords/types plus embedded element, component, and attribute markup, layered on top of VS Code's own Go grammar.
- **Diagnostics, hover, and go-to-definition** against real `.vane` source positions, backed by [`gopls`](https://pkg.go.dev/golang.org/x/tools/gopls) — `vane lsp` proxies a real `gopls` process and translates positions between `.vane` and the Go it compiles to, so you get the same tooling you'd get in a `.go` file.
- **Automatic navigation redirect**: jumping to a symbol defined in a `.vane` file (from anywhere, including plain `.go` files) lands you on the `.vane` source, not the generated Go.
- **`Vane: Restart Language Server`** and **`Vane: Open Generated Go File`** commands (command palette).

## Requirements

- The [`vane`](https://github.com/filipejohansson/vane) CLI on your `PATH`, or in your workspace root.
- [`gopls`](https://pkg.go.dev/golang.org/x/tools/gopls). The extension checks for it on activation and offers to install it (`go install golang.org/x/tools/gopls@latest`) if missing.
- The [Go extension](https://marketplace.visualstudio.com/items?itemName=golang.go) for VS Code. The extension checks for it too and offers to install it if missing — without it, embedded Go syntax inside `.vane` files won't be highlighted.

## Build and install

Not on the Marketplace yet — build and install it locally:

### Build

```bash
cd tools/vscode-vane
pnpm install                     # first time only
pnpm run package                 # builds and packages -> vscode-vane-<version>.vsix
```

### Install

**Command line:**

```bash
code --install-extension vscode-vane-<version>.vsix
```

**VS Code UI:** Extensions view (`Ctrl+Shift+X`) → `...` menu (top right) → **Install from VSIX...** → select the generated file.

Reload the VS Code window (`Developer: Reload Window` in the command palette) after installing or updating. To reinstall after making changes: repeat `pnpm run package`, then reinstall the same way (`--force` on the CLI, or just pick **Install from VSIX...** again in the UI).

## How it works

`.vane` files compile to real Go (`<Name>_vane.go`, written alongside your source). `vane lsp` spawns `gopls` against that generated Go and translates every position back and forth, so diagnostics, hover, and go-to-definition all point at your actual `.vane` source. The generated `_vane.go` files are implementation detail — they're hidden from the Explorer, and navigating into one automatically redirects you back to the `.vane` file it came from.

## Known limitations

- "Find All References" started from a plain `.go` file lists matching generated `_vane.go` files by their generated-file path, not the originating `.vane` file — clicking through still lands you on the right `.vane` source and line, but the list itself doesn't say so yet.
- Column-accurate diagnostics/hover/go-to-definition require the [Go extension](https://marketplace.visualstudio.com/items?itemName=golang.go) to be installed; without it, `.vane` files still open and edit fine, but embedded Go syntax won't be colored.

## License

MIT
