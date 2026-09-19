// Local benchmark runner: drives the four framework apps under
// benchmarks/{vane,react,svelte,solid}/dist through the same operation
// catalog js-framework-benchmark uses (create/replace/update/select/swap/
// remove/create-many/append/clear), timing each via performance.now() +
// double-requestAnimationFrame measured inside the page (not round-tripped
// through Playwright IPC), and records each app's initial transferred
// payload size. Run from the repo root: node benchmarks/runner/bench.mjs

import { chromium } from "@playwright/test";
import fs from "node:fs";
import http from "node:http";
import path from "node:path";
import { fileURLToPath } from "node:url";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const benchmarksDir = path.resolve(__dirname, "..");

// vane, vane-for, and vane-items are the same app written 3 ways, matching
// vane-page's public "Lists" docs page: vane-items uses {items()...} (compat
// path), vane-for uses native {for}+key without core.List[T], and vane uses
// core.List[T] (the fastest documented pattern). Comparing all 3 against
// React/Svelte/Solid shows the real effect of pattern choice
const APPS = [
  { name: "vane (core.List[T])", dist: path.join(benchmarksDir, "vane", "dist") },
  { name: "vane ({for}+key)", dist: path.join(benchmarksDir, "vane-for", "dist") },
  { name: "vane ({items()...})", dist: path.join(benchmarksDir, "vane-items", "dist") },
  { name: "react", dist: path.join(benchmarksDir, "react", "dist") },
  { name: "svelte", dist: path.join(benchmarksDir, "svelte", "dist") },
  { name: "solid", dist: path.join(benchmarksDir, "solid", "dist") },
];

const RUNS_PER_OPERATION = process.env.BENCH_RUNS ? parseInt(process.env.BENCH_RUNS, 10) : 10;

const MIME = {
  ".html": "text/html; charset=utf-8",
  ".js": "text/javascript; charset=utf-8",
  ".mjs": "text/javascript; charset=utf-8",
  ".css": "text/css; charset=utf-8",
  ".wasm": "application/wasm",
  ".svg": "image/svg+xml",
  ".png": "image/png",
  ".json": "application/json",
};

function serveDist(distDir) {
  const server = http.createServer((req, res) => {
    let reqPath = decodeURIComponent(req.url.split("?")[0]);
    if (reqPath === "/") reqPath = "/index.html";
    const filePath = path.join(distDir, reqPath);
    if (!filePath.startsWith(distDir)) {
      res.writeHead(403);
      res.end();
      return;
    }
    fs.readFile(filePath, (err, data) => {
      if (err) {
        res.writeHead(404, { "Content-Type": "text/plain; charset=utf-8" });
        res.end("not found");
        return;
      }
      const ext = path.extname(filePath);
      res.writeHead(200, {
        "Content-Type": MIME[ext] || "application/octet-stream",
        "Content-Length": data.length,
      });
      res.end(data);
    });
  });
  return new Promise((resolve) => {
    server.listen(0, "127.0.0.1", () => resolve(server));
  });
}

function serverPort(server) {
  return server.address().port;
}

// measureClick clicks elementId and resolves with the elapsed milliseconds
// until two consecutive requestAnimationFrame callbacks have fired
// afterward (the standard "the browser has now actually painted" signal),
// all measured with the browser's own performance.now() clock inside a
// single page.evaluate call, so Playwright IPC latency isn't part of the
// measured window.
async function measureClick(page, elementId) {
  return page.evaluate((id) => {
    return new Promise((resolve) => {
      const start = performance.now();
      document.getElementById(id).click();
      requestAnimationFrame(() => {
        requestAnimationFrame(() => {
          resolve(performance.now() - start);
        });
      });
    });
  }, elementId);
}

// measureRowClick is measureClick's counterpart for a per-row interaction,
// selecting the element via a CSS selector (row position) instead of an id,
// since row content differs per app/run.
async function measureRowClick(page, selector) {
  return page.evaluate((sel) => {
    return new Promise((resolve) => {
      const el = document.querySelector(sel);
      const start = performance.now();
      el.click();
      requestAnimationFrame(() => {
        requestAnimationFrame(() => {
          resolve(performance.now() - start);
        });
      });
    });
  }, selector);
}

const ROW2_LABEL = "#tbody tr:nth-child(2) .col-label a";
const ROW2_REMOVE = "#tbody tr:nth-child(2) .col-remove a";

// Each operation: name, a setup() run unmeasured to reach the right
// precondition state, and a measure() that performs and times the actual
// operation under test. A fresh page load precedes every operation (see
// runApp below), matching js-framework-benchmark's own practice of
// isolating each test rather than letting state/memory accumulate across
// operations and skew later results.
const OPERATIONS = [
  {
    name: "create rows (1,000)",
    setup: async () => {},
    measure: (page) => measureClick(page, "run"),
  },
  {
    name: "replace all rows",
    setup: (page) => page.click("#run"),
    measure: (page) => measureClick(page, "run"),
  },
  {
    name: "partial update (every 10th row)",
    setup: (page) => page.click("#run"),
    measure: (page) => measureClick(page, "update"),
  },
  {
    name: "select row",
    setup: (page) => page.click("#run"),
    measure: (page) => measureRowClick(page, ROW2_LABEL),
  },
  {
    name: "swap rows",
    setup: (page) => page.click("#run"),
    measure: (page) => measureClick(page, "swaprows"),
  },
  {
    name: "remove row",
    setup: (page) => page.click("#run"),
    measure: (page) => measureRowClick(page, ROW2_REMOVE),
  },
  {
    name: "create many rows (10,000)",
    setup: async () => {},
    measure: (page) => measureClick(page, "runlots"),
  },
  {
    name: "append rows (1,000 to 1,000)",
    setup: (page) => page.click("#run"),
    measure: (page) => measureClick(page, "add"),
  },
  {
    name: "clear rows",
    setup: (page) => page.click("#run"),
    measure: (page) => measureClick(page, "clear"),
  },
];

function median(values) {
  const sorted = [...values].sort((a, b) => a - b);
  const mid = Math.floor(sorted.length / 2);
  return sorted.length % 2 !== 0
    ? sorted[mid]
    : (sorted[mid - 1] + sorted[mid]) / 2;
}

async function measureInitialPayload(browser, baseUrl) {
  const page = await browser.newPage();
  let totalBytes = 0;
  page.on("response", async (response) => {
    const len = response.headers()["content-length"];
    if (len) totalBytes += parseInt(len, 10);
  });
  await page.goto(baseUrl, { waitUntil: "networkidle" });
  await page.waitForSelector("#run");
  await page.close();
  return totalBytes;
}

async function runApp(browser, app, baseUrl) {
  console.log(`\n=== ${app.name} ===`);
  const payloadBytes = await measureInitialPayload(browser, baseUrl);
  console.log(`  initial payload: ${(payloadBytes / 1024).toFixed(1)} KB`);

  const results = { app: app.name, payloadBytes, operations: {} };

  for (const op of OPERATIONS) {
    const samples = [];
    for (let i = 0; i < RUNS_PER_OPERATION; i++) {
      const page = await browser.newPage();
      await page.goto(baseUrl, { waitUntil: "networkidle" });
      await page.waitForSelector("#run");
      await op.setup(page);
      const ms = await op.measure(page);
      samples.push(ms);
      await page.close();
    }
    const m = median(samples);
    results.operations[op.name] = { median: m, samples };
    console.log(`  ${op.name.padEnd(32)} ${m.toFixed(2)} ms (median of ${RUNS_PER_OPERATION})`);
  }

  return results;
}

async function main() {
  const browser = await chromium.launch();
  const allResults = [];

  for (const app of APPS) {
    if (!fs.existsSync(app.dist)) {
      console.error(`skipping ${app.name}: no dist/ at ${app.dist} (build it first)`);
      continue;
    }
    const server = await serveDist(app.dist);
    const baseUrl = `http://127.0.0.1:${serverPort(server)}/`;
    try {
      const result = await runApp(browser, app, baseUrl);
      allResults.push(result);
    } finally {
      server.close();
    }
  }

  await browser.close();

  const outPath = path.join(benchmarksDir, "results", "results.json");
  fs.writeFileSync(
    outPath,
    JSON.stringify(
      { runAt: new Date().toISOString(), runsPerOperation: RUNS_PER_OPERATION, results: allResults },
      null,
      2,
    ),
  );
  console.log(`\nWrote ${outPath}`);
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
