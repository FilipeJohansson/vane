// Regression test: the compiler now emits `//line file:line:col` (column
// always present, see compiler.go's lineDirFor), but this parser used to
// only match the old columnless `file:line` form, silently breaking every
// "jump from a plain .go file back to .vane" redirect (falls back to line 0
// every time instead of erroring loudly).

const assert = require('node:assert');
const { vaneLineFromGoContent } = require('../linemap');

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

check('maps a go line to its vane line via a columnless //line directive (pre-1.0.0 compiler output shape)', () => {
  const go = [
    '//go:build js && wasm',
    '',
    '//line Home.vane:1',
    'package main',
    '//line Home.vane:5',
    'func F() {',
    '\tcore.Text("hi")',
    '}',
  ].join('\n');
  // go line 6 (0-indexed) is "\tcore.Text(\"hi\")", 1 line after the
  // "Home.vane:5" directive (go line 4) -> vane line 4 (0-indexed) + 1 = 5.
  assert.strictEqual(vaneLineFromGoContent(go, 6), 5);
});

check('maps a go line to its vane line via a //line:col directive (current compiler output shape)', () => {
  const go = [
    '//go:build js && wasm',
    '',
    '//line Home.vane:1:1',
    'package main',
    '//line Home.vane:5:2',
    'func F() {',
    '\tcore.Text("hi")',
    '}',
  ].join('\n');
  assert.strictEqual(vaneLineFromGoContent(go, 6), 5);
});

check('extrapolates from line 0 when no directive precedes the target line (never negative)', () => {
  const go = ['package main', 'func F() {}'].join('\n');
  assert.strictEqual(vaneLineFromGoContent(go, 0), 0);
  assert.strictEqual(vaneLineFromGoContent(go, 1), 1);
});

if (failures > 0) {
  console.error(`\n${failures} linemap test(s) failed`);
  process.exit(1);
}
console.log('\nall linemap tests passed');
