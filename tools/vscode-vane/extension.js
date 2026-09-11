'use strict';

const { LanguageClient, TransportKind } = require('vscode-languageclient/node');
const vscode = require('vscode');
const { workspace } = vscode;
const { execSync } = require('child_process');
const path = require('path');
const fs = require('fs');
const { checkGoplsPresent } = require('./gopls');

const GO_EXTENSION_ID = 'golang.go';

let client;
let output;

// Files the redirect listener should skip once, because the user just asked
// (via vane.openGeneratedGoFile) to view the generated file directly.
const suppressRedirect = new Set();

function log(msg) {
  if (output) output.appendLine(msg);
}

function resolveVaneBin() {
  // Try workspace-local binary first, then PATH.
  const workspaceFolders = workspace.workspaceFolders;
  if (workspaceFolders && workspaceFolders.length > 0) {
    const root = workspaceFolders[0].uri.fsPath;
    const candidates = [
      path.join(root, 'vane'),
      path.join(root, 'vane.exe'),
    ];
    for (const c of candidates) {
      try {
        require('fs').accessSync(c, require('fs').constants.X_OK);
        return c;
      } catch (_) {}
    }
  }
  // Fall back to PATH.
  try {
    const result = execSync(process.platform === 'win32' ? 'where vane' : 'which vane', { encoding: 'utf8' });
    return result.trim().split('\n')[0];
  } catch (_) {
    return 'vane';
  }
}

function makeClient() {
  const vaneBin = resolveVaneBin();

  const serverOptions = {
    command: vaneBin,
    args: ['lsp'],
    transport: TransportKind.stdio,
  };

  const clientOptions = {
    documentSelector: [{ scheme: 'file', language: 'vane' }],
    synchronize: {
      fileEvents: workspace.createFileSystemWatcher('**/*.vane'),
    },
  };

  return new LanguageClient('vane', 'Vane Language Server', serverOptions, clientOptions);
}

// ensureTooling prompts to install gopls and/or the Go extension if either
// is missing, matching vscode-go's own pattern for gopls rather than failing
// silently. Runs in the background; doesn't block activation.
async function ensureTooling() {
  if (!checkGoplsPresent(execSync)) {
    const choice = await vscode.window.showWarningMessage(
      'Vane: gopls not found on PATH. The language server needs it for diagnostics, hover, and go-to-definition.',
      'Install gopls',
    );
    if (choice === 'Install gopls') {
      const term = vscode.window.createTerminal('Vane: install gopls');
      term.show();
      term.sendText('go install golang.org/x/tools/gopls@latest');
    }
  }

  if (!vscode.extensions.getExtension(GO_EXTENSION_ID)) {
    const choice = await vscode.window.showWarningMessage(
      "Vane: the Go extension isn't installed. Without it, .go syntax highlighting won't work, including inside .vane files.",
      'Install Go extension',
    );
    if (choice === 'Install Go extension') {
      await vscode.commands.executeCommand('workbench.extensions.installExtension', GO_EXTENSION_ID);
    }
  }
}

function activate(context) {
  output = vscode.window.createOutputChannel('Vane');
  context.subscriptions.push(output);

  ensureTooling().catch((err) => log(`[ensureTooling] ${err}`));

  client = makeClient();
  client.start();

  context.subscriptions.push(
    vscode.commands.registerCommand('vane.restartLanguageServer', async () => {
      if (client) {
        await client.stop();
      }
      client = makeClient();
      await client.start();
      vscode.window.showInformationMessage('Vane: language server restarted.');
    })
  );

  // Escape hatch: open the generated _vane.go file directly (pinned), bypassing
  // the auto-redirect below, for when you actually want to inspect the generated code.
  context.subscriptions.push(
    vscode.commands.registerCommand('vane.openGeneratedGoFile', async () => {
      const editor = vscode.window.activeTextEditor;
      if (!editor) return;
      const file = editor.document.fileName;

      let goFile;
      if (file.endsWith('.vane')) {
        goFile = file.replace(/\.vane$/, '_vane.go');
      } else if (file.endsWith('_vane.go')) {
        goFile = file;
      } else {
        vscode.window.showWarningMessage('Vane: no generated Go file for the active editor.');
        return;
      }

      if (!fs.existsSync(goFile)) {
        vscode.window.showWarningMessage(`Vane: ${path.basename(goFile)} does not exist.`);
        return;
      }

      suppressRedirect.add(goFile);
      await vscode.window.showTextDocument(vscode.Uri.file(goFile), {
        viewColumn: editor.viewColumn,
        preview: false,
        preserveFocus: false,
      });
    })
  );

  // When any navigation (gopls "go to definition"/"go to references" from a
  // plain .go file like search.go, ctrl+click from a .vane file, etc.) lands
  // on a _vane.go file, redirect to the corresponding .vane source at the
  // mapped line. This has to be origin-agnostic: navigation from a plain .go
  // file (main.go, search.go, ...) is handled entirely by VS Code's own Go
  // extension and its own separate gopls, which never goes through vane's LSP
  // proxy at all, so there's no opportunity to translate the URI there — this
  // listener, watching the active editor itself, is the only hook available.
  //
  // Always redirect, unless suppressed via vane.openGeneratedGoFile. Used to
  // skip redirecting on an empty selection (assuming that meant "deliberate
  // open"), but plain ctrl+click go-to-definition onto a declaration also
  // leaves selection empty, so it never redirected. _vane.go is also
  // Explorer-hidden (files.exclude), so accidental opens are rare anyway.
  context.subscriptions.push(
    vscode.window.onDidChangeActiveTextEditor(editor => {
      if (!editor) return;
      const file = editor.document.fileName;
      if (!file.endsWith('_vane.go')) return;

      log(`[activeEditor] file=${file} suppressed=${suppressRedirect.has(file)}`);

      // editor.selection isn't necessarily at the real navigation target
      // yet at this exact event (confirmed live: ctrl+click go-to-definition
      // always read back as 0,0 here), so wait for the selection VS Code
      // sets right after activating the editor, with a timeout fallback for
      // opens that never fire a follow-up selection change (0,0 is then the
      // real target).
      let settled = false;
      const sub = vscode.window.onDidChangeTextEditorSelection(e => {
        if (settled || e.textEditor !== editor) return;
        settled = true;
        clearTimeout(timer);
        sub.dispose();
        maybeRedirectVaneGoFile(file, editor);
      });
      const timer = setTimeout(() => {
        if (settled) return;
        settled = true;
        sub.dispose();
        maybeRedirectVaneGoFile(file, editor);
      }, 150);
    })
  );
}

async function maybeRedirectVaneGoFile(file, editor) {
  if (suppressRedirect.delete(file)) {
    log(`[redirect] SKIP: ${file} suppressed (vane.openGeneratedGoFile)`);
    return;
  }
  if (!client) {
    log(`[redirect] SKIP: no language client running`);
    return;
  }

  // Ask the running vane lsp server to translate this generated-file
  // position back to .vane, using its real SourceMap, the file on disk
  // never has //line directives (writeToDisk strips them so gopls isn't
  // confused by them), so there's nothing to parse client-side.
  const active = editor.selection.active;
  let result;
  try {
    result = await client.sendRequest('vane/goToVanePosition', {
      uri: editor.document.uri.toString(),
      position: { line: active.line, character: active.character },
    });
  } catch (err) {
    log(`[redirect] vane/goToVanePosition request failed: ${err}`);
    return;
  }
  if (!result) {
    log(`[redirect] SKIP: no vane mapping for ${file}:${active.line}:${active.character}`);
    return;
  }

  log(`[redirect] REDIRECTING ${file} -> ${result.uri}:${result.position.line}`);
  const vaneUri = vscode.Uri.parse(result.uri);
  const pos = new vscode.Position(result.position.line, result.position.character);

  await vscode.window.showTextDocument(vaneUri, {
    viewColumn: editor.viewColumn,
    selection: new vscode.Range(pos, pos),
    preserveFocus: false,
  });

  // Close the _vane.go tab. Use tab groups API (VS Code 1.76+) when available.
  try {
    if (vscode.window.tabGroups && vscode.window.tabGroups.all) {
      for (const group of vscode.window.tabGroups.all) {
        for (const tab of group.tabs) {
          if (tab.input && tab.input.uri && tab.input.uri.fsPath === file) {
            await vscode.window.tabGroups.close(tab);
            return;
          }
        }
      }
    }
  } catch (_) {}
}

function deactivate() {
  if (client) {
    return client.stop();
  }
}

module.exports = { activate, deactivate };
