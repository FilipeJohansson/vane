'use strict';

// Parse //line directives from a _vane.go file to map a go line (0-indexed)
// back to the corresponding .vane source line (0-indexed). The compiler
// emits `//line file:line:col` (column always present, compiler.go's
// lineDirFor), but the column isn't needed here (this maps whole lines for
// tab redirection, not cursor position), so it's matched optionally to stay
// compatible with either form.
//
// Kept in its own module, separate from extension.js, so it can be unit
// tested with plain Node - extension.js requires 'vscode', a module that
// only exists inside a running VS Code extension host.
function vaneLineFromGoContent(goContent, goLine) {
  const lines = goContent.split('\n');
  let lastVaneLine = 0;
  let lastGoLine = 0;
  for (let i = 0; i <= goLine && i < lines.length; i++) {
    const m = lines[i].match(/^\/\/line [^:]+:(\d+)(?::\d+)?\s*$/);
    if (m) {
      lastVaneLine = parseInt(m[1], 10) - 1; // convert 1-indexed to 0-indexed
      lastGoLine = i + 1; // code starts at the line after the directive
    }
  }
  return Math.max(0, lastVaneLine + (goLine - lastGoLine));
}

module.exports = { vaneLineFromGoContent };
