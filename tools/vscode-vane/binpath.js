'use strict';

// firstPathLine returns the first non-empty line of `where`/`which` output,
// trimmed. Splits on CRLF or LF before trimming, so a Windows `where vane`
// with several matches doesn't leave a trailing '\r' on the first path.
function firstPathLine(output) {
  return (
    String(output)
      .split(/\r?\n/)
      .map((l) => l.trim())
      .find((l) => l !== '') || ''
  );
}

module.exports = { firstPathLine };
