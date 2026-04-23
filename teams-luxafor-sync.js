#!/usr/bin/env node

const fs = require('fs');
const path = require('path');
const { device } = require('luxafor-api');

const HOME = process.env.HOME;
const POLL_MS = Number(process.env.POLL_MS || 3000);
const TAIL_BYTES = Number(process.env.TAIL_BYTES || 256 * 1024);
const FALLBACK_LOG_SCAN_COUNT = Number(process.env.FALLBACK_LOG_SCAN_COUNT || 5);
const REAPPLY_MS = Number(process.env.REAPPLY_MS || 15000);

const BUSY_STATUSES = new Set([
  'busy',
  'donotdisturb',
  'inacall',
  'inaconferencecall',
  'inameeting',
  'presenting',
  'focusing',
  'berightback',
]);

let luxafor = null;
let lastState = null;
let lastColor = null;
let lastLogFile = null;
let cachedLogDir = null;
let lastNoLogMessageAt = 0;
let lastApplyAt = 0;

function now() {
  return new Date().toISOString();
}

function listLogCandidates() {
  const direct = [];
  const groupContainersRoot = path.join(HOME, 'Library/Group Containers');

  if (process.env.TEAMS_LOG_DIR) {
    direct.push(process.env.TEAMS_LOG_DIR);
  }

  direct.push(
    path.join(
      HOME,
      'Library/Group Containers/UBF8T346G9.com.microsoft.teams/Library/Application Support/Logs'
    ),
    path.join(HOME, 'Library/Application Support/Microsoft/Teams/logs')
  );

  try {
    const groupDirs = fs.readdirSync(groupContainersRoot);
    for (const entry of groupDirs) {
      if (!entry.toLowerCase().includes('microsoft.teams')) {
        continue;
      }
      direct.push(path.join(groupContainersRoot, entry, 'Library/Application Support/Logs'));
    }
  } catch (_err) {
    // ignore
  }

  return [...new Set(direct)];
}

function hasTeamsLog(dirPath) {
  try {
    return fs.readdirSync(dirPath).some((name) => name.startsWith('MSTeams_') && name.endsWith('.log'));
  } catch (_err) {
    return false;
  }
}

function resolveLogDir() {
  if (cachedLogDir && hasTeamsLog(cachedLogDir)) {
    return cachedLogDir;
  }

  const candidates = listLogCandidates();
  for (const dirPath of candidates) {
    if (hasTeamsLog(dirPath)) {
      cachedLogDir = dirPath;
      return dirPath;
    }
  }

  return null;
}

function getTeamsLogFilesSorted() {
  const logDir = resolveLogDir();
  if (!logDir) {
    return [];
  }

  return fs
    .readdirSync(logDir)
    .filter((name) => name.startsWith('MSTeams_') && name.endsWith('.log'))
    .map((name) => {
      const fullPath = path.join(logDir, name);
      const stat = fs.statSync(fullPath);
      return { fullPath, mtimeMs: stat.mtimeMs };
    })
    .sort((a, b) => b.mtimeMs - a.mtimeMs);
}

function getLatestTeamsLogFile() {
  const files = getTeamsLogFilesSorted();
  return files.length ? files[0].fullPath : null;
}

function readTail(filePath, bytes) {
  const stat = fs.statSync(filePath);
  const start = Math.max(0, stat.size - bytes);
  const length = stat.size - start;

  const fd = fs.openSync(filePath, 'r');
  try {
    const buffer = Buffer.alloc(length);
    fs.readSync(fd, buffer, 0, length, start);
    return buffer.toString('utf8');
  } finally {
    fs.closeSync(fd);
  }
}

function extractAvailability(logChunk) {
  const regex = /availability:\s*([A-Za-z]+)/g;
  let match;
  let value = null;

  while ((match = regex.exec(logChunk)) !== null) {
    value = match[1];
  }

  return value;
}

function findMostRecentAvailability() {
  const files = getTeamsLogFilesSorted().slice(0, FALLBACK_LOG_SCAN_COUNT);
  for (const file of files) {
    try {
      const chunk = readTail(file.fullPath, TAIL_BYTES);
      const availability = extractAvailability(chunk);
      if (availability) {
        return availability;
      }
    } catch (_err) {
      // ignore per-file failures and continue
    }
  }
  return null;
}

function mapToColor(statusRaw) {
  const status = String(statusRaw || '').toLowerCase();
  return BUSY_STATUSES.has(status) ? 'red' : 'green';
}

function getLuxafor() {
  if (!luxafor) {
    luxafor = device();
  }
  return luxafor;
}

function setLuxaforColor(color) {
  try {
    getLuxafor().color(color);
    return true;
  } catch (err) {
    luxafor = null;
    console.error(`[${now()}] Luxafor error: ${err.message}`);
    return false;
  }
}

function tick() {
  try {
    const logDir = resolveLogDir();
    const logFile = getLatestTeamsLogFile();
    if (!logDir || !logFile) {
      const nowMs = Date.now();
      if (nowMs - lastNoLogMessageAt > 60_000) {
        console.error(`[${now()}] No Teams logs found. Set TEAMS_LOG_DIR if needed.`);
        lastNoLogMessageAt = nowMs;
      }
      return;
    }

    if (lastLogFile !== logFile) {
      lastLogFile = logFile;
      console.log(`[${now()}] Using log directory: ${logDir}`);
      console.log(`[${now()}] Following ${logFile}`);
    }

    const chunk = readTail(logFile, TAIL_BYTES);
    let availability = extractAvailability(chunk);
    if (!availability) {
      availability = findMostRecentAvailability();
    }
    if (!availability) {
      return;
    }

    const color = mapToColor(availability);
    const nowMs = Date.now();
    const needsPeriodicReapply = nowMs - lastApplyAt >= REAPPLY_MS;

    if (availability !== lastState || color !== lastColor || needsPeriodicReapply) {
      const changed = setLuxaforColor(color);
      if (changed) {
        lastState = availability;
        lastColor = color;
        lastApplyAt = nowMs;
        console.log(`[${now()}] Teams=${availability} -> Luxafor=${color}`);
      }
    }
  } catch (err) {
    console.error(`[${now()}] Sync error: ${err.message}`);
  }
}

console.log(`[${now()}] Teams -> Luxafor sync started`);
tick();
setInterval(tick, POLL_MS);
