'use strict';

const { LanguageClient, TransportKind } = require('vscode-languageclient/node');
const vscode = require('vscode');
const { workspace } = vscode;
const { execSync } = require('child_process');
const path = require('path');
const fs = require('fs');

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

// Parse //line directives from a _vane.go file to map a go line (0-indexed)
// back to the corresponding .vane source line (0-indexed).
function vaneLineFromGoContent(goContent, goLine) {
  const lines = goContent.split('\n');
  let lastVaneLine = 0;
  let lastGoLine = 0;
  for (let i = 0; i <= goLine && i < lines.length; i++) {
    const m = lines[i].match(/^\/\/line [^:]+:(\d+)\s*$/);
    if (m) {
      lastVaneLine = parseInt(m[1], 10) - 1; // convert 1-indexed to 0-indexed
      lastGoLine = i + 1; // code starts at the line after the directive
    }
  }
  return Math.max(0, lastVaneLine + (goLine - lastGoLine));
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

function activate(context) {
  output = vscode.window.createOutputChannel('Vane');
  context.subscriptions.push(output);

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
  // Distinguish "the editor navigated here" from "I opened this file myself"
  // by selection, not tab-preview state: every navigation source (go to
  // definition, ctrl+click, clicking a reference in the References panel)
  // selects the target symbol's range, a non-empty selection. Deliberately
  // opening a file (double-click in Explorer, Quick Open, a restored tab)
  // just places the cursor, an empty selection. This also fixed a case
  // tab-preview state got wrong: double-clicking a reference in the
  // References panel opens the file pinned (not preview, same as opening it
  // deliberately), which used to skip the redirect entirely.
  context.subscriptions.push(
    vscode.window.onDidChangeActiveTextEditor(editor => {
      if (!editor) return;
      const file = editor.document.fileName;
      if (!file.endsWith('_vane.go')) return;

      const tab = vscode.window.tabGroups.activeTabGroup.activeTab;
      log(
        `[activeEditor] file=${file} ` +
        `selection.isEmpty=${editor.selection.isEmpty} ` +
        `selection=${editor.selection.start.line}:${editor.selection.start.character}-${editor.selection.end.line}:${editor.selection.end.character} ` +
        `isPreview=${tab ? tab.isPreview : 'n/a'} ` +
        `suppressed=${suppressRedirect.has(file)}`
      );

      if (editor.selection.isEmpty) {
        log(`[activeEditor] SKIP: empty selection, treating as deliberate open`);
        return;
      }

      maybeRedirectVaneGoFile(file, editor);
    })
  );
}

async function maybeRedirectVaneGoFile(file, editor) {
  if (suppressRedirect.delete(file)) {
    log(`[redirect] SKIP: ${file} suppressed (vane.openGeneratedGoFile)`);
    return;
  }

  const vaneFile = file.replace(/_vane\.go$/, '.vane');
  if (!fs.existsSync(vaneFile)) {
    log(`[redirect] SKIP: no matching .vane file for ${file} (looked for ${vaneFile})`);
    return;
  }

  log(`[redirect] REDIRECTING ${file} -> ${vaneFile}`);
  const goLine = editor.selection.active.line;
  const vaneLine = vaneLineFromGoContent(editor.document.getText(), goLine);
  const vaneUri = vscode.Uri.file(vaneFile);
  const pos = new vscode.Position(vaneLine, 0);

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
