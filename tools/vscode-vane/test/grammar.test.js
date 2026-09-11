// Tokenizes sample .vane lines through the real TextMate grammar
// (syntaxes/vane.tmLanguage.json + syntaxes/vane-jsx.tmLanguage.json) using
// vscode-textmate/vscode-oniguruma, the same engine VS Code itself uses, and
// asserts the scopes VS Code's theming/semantic layers key off of. This
// doesn't need a running VS Code instance.
//
// A trivial (empty) stand-in is registered for `source.go` rather than a
// real Go grammar. This isn't a shortcut: `getInjections`/nested-include
// resolution behaves differently in vscode-textmate depending on whether
// every referenced scope resolves to *something*, even an empty grammar,
// an unresolved include corrupts sibling-rule matching elsewhere in the
// same grammar (confirmed by hand: removing the stand-in reproduces a
// completely different, wrong tokenization for attribute expressions,
// vs. leaving it in). A real VS Code install always has this resolved
// (bundled, or via the Go extension almost every Go/Vane user already has
// for gopls itself); the case worth testing defensively is a user without
// the Go extension installed at all, where `source.go` would ALSO be
// unresolved in real VS Code, this suite's setup matches that worst case.

const assert = require('node:assert');
const fs = require('node:fs');
const path = require('node:path');
const oniguruma = require('vscode-oniguruma');
const { Registry, INITIAL } = require('vscode-textmate');

const GRAMMAR_DIR = path.join(__dirname, '..', 'syntaxes');
const GRAMMARS = {
  'source.vane': path.join(GRAMMAR_DIR, 'vane.tmLanguage.json'),
  'source.vane.jsx': path.join(GRAMMAR_DIR, 'vane-jsx.tmLanguage.json'),
};
// Empty stand-in for the Go extension's own grammar - see the file comment.
const TRIVIAL_GO_GRAMMAR = { scopeName: 'source.go', patterns: [] };

async function createRegistry() {
  const wasmPath = path.join(
    path.dirname(require.resolve('vscode-oniguruma/package.json')),
    'release',
    'onig.wasm',
  );
  await oniguruma.loadWASM(fs.readFileSync(wasmPath).buffer);
  const onigLib = Promise.resolve({
    createOnigScanner: (patterns) => new oniguruma.OnigScanner(patterns),
    createOnigString: (s) => new oniguruma.OnigString(s),
  });
  return new Registry({
    onigLib,
    loadGrammar: async (scopeName) => {
      if (scopeName === 'source.go') return TRIVIAL_GO_GRAMMAR;
      const file = GRAMMARS[scopeName];
      if (!file) return null;
      return JSON.parse(fs.readFileSync(file, 'utf8'));
    },
    // vscode-textmate doesn't read package.json's "injectTo" - that's a VS
    // Code extension-host concept. Standalone, injections must be declared
    // here, mirroring what package.json's contributes.grammars says.
    getInjections: (scopeName) => (scopeName === 'source.vane' ? ['source.vane.jsx'] : undefined),
  });
}

// tokenizeLine returns the list of {text, scopes} tokens for one line,
// tokenized in isolation (a fresh INITIAL stack) unless a prior stack is
// passed in, needed for multi-line constructs like an open <div> whose
// attributes/children span several lines.
function tokenizeLine(grammar, line, prevState) {
  const result = grammar.tokenizeLine(line, prevState || INITIAL);
  const tokens = result.tokens.map((t) => ({
    text: line.slice(t.startIndex, t.endIndex),
    scopes: t.scopes,
  }));
  return { tokens, ruleStack: result.ruleStack };
}

function scopesAt(tokens, substring) {
  const t = tokens.find((t) => t.text.includes(substring));
  assert.ok(t, `no token contains ${JSON.stringify(substring)} in tokens: ${JSON.stringify(tokens.map((t) => t.text))}`);
  return t.scopes;
}

function hasScope(scopes, wanted) {
  return scopes.some((s) => s.includes(wanted));
}

async function main() {
  const registry = await createRegistry();
  const grammar = await registry.loadGrammar('source.vane');
  assert.ok(grammar, 'failed to load source.vane grammar');

  let failures = 0;
  function check(name, fn) {
    try {
      fn();
      console.log(`ok - ${name}`);
    } catch (err) {
      failures++;
      console.error(`FAIL - ${name}\n  ${err.message}`);
    }
  }

  check('lowercase element tag name gets entity.name.tag.html', () => {
    const { tokens } = tokenizeLine(grammar, '<div className="app">');
    const scopes = scopesAt(tokens, 'div');
    assert.ok(hasScope(scopes, 'entity.name.tag.html'), `scopes: ${scopes}`);
  });

  check('quoted attribute: name/=/value each get their own scope', () => {
    const { tokens } = tokenizeLine(grammar, '<div className="app">');
    assert.ok(hasScope(scopesAt(tokens, 'className'), 'entity.other.attribute-name'));
    assert.ok(hasScope(scopesAt(tokens, '"app"'), 'string.quoted.double'));
  });

  check('expression attribute: name/braces get their own scope', () => {
    const { tokens } = tokenizeLine(grammar, '<div onClick={handleClick}>');
    assert.ok(hasScope(scopesAt(tokens, 'onClick'), 'entity.other.attribute-name'));
    const openBrace = tokens.find((t) => t.text === '{');
    assert.ok(openBrace, 'no { token found');
    assert.ok(hasScope(openBrace.scopes, 'punctuation.section.embedded.begin'), `scopes: ${openBrace.scopes}`);
  });

  check('bare attribute (no "=") gets entity.other.attribute-name', () => {
    const { tokens } = tokenizeLine(grammar, '<input disabled>');
    assert.ok(hasScope(scopesAt(tokens, 'disabled'), 'entity.other.attribute-name'));
  });

  check('fragment open tag gets punctuation.definition.tag, no entity.name.tag', () => {
    const { tokens } = tokenizeLine(grammar, '<>');
    const lt = tokens.find((t) => t.text === '<');
    assert.ok(lt, 'no < token found');
    assert.ok(hasScope(lt.scopes, 'punctuation.definition.tag.begin.vane'), `scopes: ${lt.scopes}`);
    assert.ok(!tokens.some((t) => hasScope(t.scopes, 'entity.name.tag.html')), 'fragment should not carry a tag-name scope');
  });

  check('uppercase component reference is NOT scoped as an element tag (it\'s a plain Go call per the compiler)', () => {
    const { tokens } = tokenizeLine(grammar, '<Card title="x"/>');
    const cardToken = tokens.find((t) => t.text.includes('Card'));
    assert.ok(cardToken, 'no Card token found');
    assert.ok(!hasScope(cardToken.scopes, 'entity.name.tag.html'), `Card incorrectly scoped as a tag: ${cardToken.scopes}`);
  });

  check('a nested element inside an embedded {for ...{ ... }} block still gets tag scopes (L: injection re-triggers)', () => {
    let { tokens, ruleStack } = tokenizeLine(grammar, '{for _, item := range items {');
    ({ tokens, ruleStack } = tokenizeLine(grammar, '<li>{item}</li>', ruleStack));
    const scopes = scopesAt(tokens, 'li');
    assert.ok(hasScope(scopes, 'entity.name.tag.html'), `scopes: ${scopes}`);
  });

  check('two attributes on one line each resolve to their own attribute-name scope (not just the last one)', () => {
    const { tokens } = tokenizeLine(grammar, '<div id="a" onClick={handleClick}>');
    assert.ok(hasScope(scopesAt(tokens, 'id'), 'entity.other.attribute-name'));
    assert.ok(hasScope(scopesAt(tokens, 'onClick'), 'entity.other.attribute-name'));
  });

  if (failures > 0) {
    console.error(`\n${failures} grammar test(s) failed`);
    process.exit(1);
  }
  console.log(`\nall grammar tests passed`);
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
