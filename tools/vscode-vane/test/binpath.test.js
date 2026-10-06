const assert = require('node:assert');
const { firstPathLine } = require('../binpath');

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

check('single match with trailing CRLF has no carriage return', () => {
  assert.strictEqual(firstPathLine('C:\go\bin\vane.exe\r\n'), 'C:\go\bin\vane.exe');
});

check('multiple CRLF matches return the first without a carriage return', () => {
  assert.strictEqual(
    firstPathLine('C:\go\bin\vane.exe\r\nC:\other\vane.cmd\r\n'),
    'C:\go\bin\vane.exe',
  );
});

check('LF-separated matches return the first', () => {
  assert.strictEqual(firstPathLine('/usr/local/bin/vane\n/usr/bin/vane\n'), '/usr/local/bin/vane');
});

check('empty output returns an empty string', () => {
  assert.strictEqual(firstPathLine(''), '');
});

if (failures > 0) process.exit(1);
