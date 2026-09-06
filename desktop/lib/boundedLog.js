'use strict';

// A server's stdout/stderr is the only diagnostic a user can send us, so it
// goes to a real file — capped, keeping the tail, since a long-running
// session with a chatty CLI can otherwise fill a disk.
//
// Shared by every AgentWorks-family desktop shell.

const fs = require('fs');
const path = require('path');

const DEFAULT_MAX_BYTES = 25 * 1024 * 1024;
const DEFAULT_KEEP_RATIO = 0.75;

function toLogBuffer(chunk) {
  if (Buffer.isBuffer(chunk)) return chunk;
  if (chunk instanceof Uint8Array) return Buffer.from(chunk);
  return Buffer.from(String(chunk));
}

// trimLogFileToTail shrinks filePath to its tail when it is over maxBytes,
// prefixing a one-line header that says so. Returns the resulting size.
function trimLogFileToTail(filePath, { maxBytes = DEFAULT_MAX_BYTES, keepRatio = DEFAULT_KEEP_RATIO, keepBytes = null, appName = 'AgentWorks' } = {}) {
  if (!maxBytes || maxBytes <= 0) return 0;
  let stat;
  try { stat = fs.statSync(filePath); } catch (_e) { return 0; }
  if (!stat.isFile()) return 0;

  const targetKeepBytes = keepBytes == null ? Math.floor(maxBytes * keepRatio) : Math.max(0, keepBytes);
  if (stat.size <= maxBytes && stat.size <= targetKeepBytes) return stat.size;

  const header = Buffer.from(`[${new Date().toISOString()}] Log truncated by ${appName} to stay under ${maxBytes} bytes; kept the tail of a ${stat.size} byte file.\n`);
  const readBytes = Math.min(targetKeepBytes, Math.max(0, maxBytes - header.length), stat.size);
  let tail = Buffer.alloc(0);
  if (readBytes > 0) {
    const fd = fs.openSync(filePath, 'r');
    try {
      tail = Buffer.allocUnsafe(readBytes);
      fs.readSync(fd, tail, 0, readBytes, stat.size - readBytes);
    } finally {
      fs.closeSync(fd);
    }
  }
  const nextContent = Buffer.concat([header, tail]);
  fs.writeFileSync(filePath, nextContent, { mode: 0o600 });
  return nextContent.length;
}

// createBoundedLogWriter returns { file, write(chunk), end() } appending to
// filePath and never letting it grow past maxBytes. Synchronous by design:
// the writer must never take the app down, and a crash right after a write
// must not lose it.
function createBoundedLogWriter(filePath, { maxBytes = DEFAULT_MAX_BYTES, keepRatio = DEFAULT_KEEP_RATIO, appName = 'AgentWorks' } = {}) {
  fs.mkdirSync(path.dirname(filePath), { recursive: true });
  let currentSize = trimLogFileToTail(filePath, { maxBytes, keepRatio, appName });

  return {
    file: filePath,
    write(chunk) {
      const buffer = toLogBuffer(chunk);
      if (buffer.length === 0) return;
      try {
        if (maxBytes > 0 && buffer.length >= maxBytes) {
          const header = Buffer.from(`[${new Date().toISOString()}] Oversized log chunk truncated by ${appName}; kept the final bytes of a ${buffer.length} byte write.\n`);
          const tailBudget = Math.max(0, maxBytes - header.length);
          const tail = buffer.subarray(Math.max(0, buffer.length - tailBudget));
          const nextContent = Buffer.concat([header, tail]);
          fs.writeFileSync(filePath, nextContent, { mode: 0o600 });
          currentSize = nextContent.length;
          return;
        }
        if (maxBytes > 0 && currentSize + buffer.length > maxBytes) {
          currentSize = trimLogFileToTail(filePath, { maxBytes, keepRatio, keepBytes: Math.max(0, maxBytes - buffer.length - 1024), appName });
        }
        fs.appendFileSync(filePath, buffer, { mode: 0o600 });
        currentSize += buffer.length;
      } catch (err) {
        console.warn('[main] Failed to write bounded log:', err && err.message ? err.message : err);
      }
    },
    end() {
      // Synchronous writer; retained for compatibility with stream-like call sites.
    },
  };
}

module.exports = { DEFAULT_MAX_BYTES, DEFAULT_KEEP_RATIO, toLogBuffer, trimLogFileToTail, createBoundedLogWriter };
