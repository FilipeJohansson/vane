'use strict';

// checkGoplsPresent reports whether `gopls` runs successfully on PATH.
// execFn is injected (execSync from 'child_process' in real use) so this can
// be unit tested with plain Node without pulling in 'vscode'.
function checkGoplsPresent(execFn) {
  try {
    execFn('gopls version', { stdio: 'ignore' });
    return true;
  } catch (_) {
    return false;
  }
}

module.exports = { checkGoplsPresent };
