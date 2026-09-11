const assert = require('node:assert');
const { checkGoplsPresent } = require('../gopls');

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

check('reports true when execFn succeeds (gopls found and runs)', () => {
  const result = checkGoplsPresent(() => Buffer.from('gopls v0.16.0'));
  assert.strictEqual(result, true);
});

check('reports false when execFn throws (gopls missing or not runnable)', () => {
  const result = checkGoplsPresent(() => {
    throw new Error('spawn gopls ENOENT');
  });
  assert.strictEqual(result, false);
});

check('calls execFn with a "gopls version" command', () => {
  let calledWith = null;
  checkGoplsPresent((cmd) => {
    calledWith = cmd;
    return Buffer.from('');
  });
  assert.strictEqual(calledWith, 'gopls version');
});

if (failures > 0) {
  console.error(`\n${failures} gopls test(s) failed`);
  process.exit(1);
}
console.log('\nall gopls tests passed');
