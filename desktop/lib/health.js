'use strict';

// Readiness polling for the two local servers every AgentWorks-family desktop
// shell spawns. Shared so a timing or error-message fix lands once.

const http = require('http');

function fetchHealth(url, timeoutMs = 5000) {
  return new Promise((resolve) => {
    const req = http.get(url, (res) => { res.resume(); resolve(res.statusCode === 200); });
    req.on('error', () => resolve(false));
    req.setTimeout(timeoutMs, () => { req.destroy(); resolve(false); });
  });
}

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

// waitForHealth resolves once every check answers 200. `checks` is
// [{ name, url }]; `isAlive()` (optional) returns a string reason when a
// server has already exited, which fails fast instead of waiting out the
// timeout; `hint` is appended to the timeout error.
async function waitForHealth({ checks, timeoutMs = 90000, pollMs = 500, initialDelayMs = 0, isAlive = null, hint = '' }) {
  if (initialDelayMs > 0) await sleep(initialDelayMs);
  const deadline = Date.now() + timeoutMs;
  for (;;) {
    if (isAlive) {
      const dead = isAlive();
      if (dead) throw new Error(dead);
    }
    const results = await Promise.all(checks.map((c) => fetchHealth(c.url)));
    if (results.every(Boolean)) return;
    if (Date.now() > deadline) {
      const notReady = checks.filter((_c, i) => !results[i]).map((c) => c.name);
      const which = notReady.length ? notReady.join(' and ') : 'one or both';
      throw new Error(`Servers did not become ready in time. Not ready: ${which}.${hint ? ' ' + hint : ''}`);
    }
    await sleep(pollMs);
  }
}

module.exports = { fetchHealth, waitForHealth };
