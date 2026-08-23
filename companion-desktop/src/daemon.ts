// daemon.ts — spawn+handshake+lifecycle logic for the companiond child
// process. Deliberately free of any `electron` import: this module is
// plain Node so it can be exercised headlessly (see test/daemon.test.mjs)
// without a display or the electron binary.
//
// Contract with cmd/companiond (see companion-cutover.md phases 1-2 and
// cmd/companiond/main.go's header comment):
//   - stdout's first line, newline-terminated, is the bare "host:port"
//     the daemon bound. Nothing else structured follows on stdout.
//   - every request needs `Authorization: Bearer <token>` except
//     GET /healthz.
//   - closing the child's stdin is the primary shutdown signal; the
//     daemon exits on its own once it sees EOF. We still escalate to
//     SIGTERM/SIGKILL after a grace period in case it doesn't.

import { spawn, ChildProcessWithoutNullStreams } from "node:child_process";
import * as crypto from "node:crypto";
import * as path from "node:path";

export interface DaemonHandle {
  child: ChildProcessWithoutNullStreams;
  baseUrl: string;
  token: string;
}

export interface SpawnOptions {
  /** Override the daemon binary path (Phase 5 packaging seam). */
  companiondBin?: string;
  /** Repo root, used to locate cmd/companiond for `go run` in dev. */
  repoRoot?: string;
  /** Milliseconds to wait for the address line before giving up. */
  handshakeTimeoutMs?: number;
  /** Extra env vars merged over process.env for the child. */
  env?: NodeJS.ProcessEnv;
}

const DEFAULT_HANDSHAKE_TIMEOUT_MS = 15_000;

/** Cryptographically random bearer token, hex-encoded. */
export function generateToken(): string {
  return crypto.randomBytes(32).toString("hex");
}

/**
 * Resolves how to launch companiond. Dev default: `go run ./cmd/companiond`
 * from the repo root (two levels up from this package by default, since
 * companion-desktop/ sits alongside cmd/ in the repo). COMPANION_BIN (or
 * the companiondBin option) points at a prebuilt binary instead — the
 * seam Phase 5's app.isPackaged branching will extend to
 * process.resourcesPath.
 */
export function resolveDaemonCommand(opts: SpawnOptions = {}): {
  cmd: string;
  args: string[];
  cwd: string;
} {
  const bin = opts.companiondBin ?? process.env.COMPANION_BIN ?? process.env.COMPANIOND_BIN;
  const repoRoot = opts.repoRoot ?? path.resolve(__dirname, "..", "..");
  if (bin) {
    return { cmd: bin, args: [], cwd: repoRoot };
  }
  return { cmd: "go", args: ["run", "./cmd/companiond"], cwd: repoRoot };
}

/**
 * Spawns companiond with a fresh token in its environment, reads the
 * handshake line off stdout, and resolves once the address is known.
 * Remaining stdout is drained (ignored, per the contract — only the
 * first line is structured); stderr is forwarded line-by-line to
 * `onLog` for the shell's own log.
 */
export function spawnDaemon(
  opts: SpawnOptions & { onLog?: (line: string) => void } = {}
): Promise<DaemonHandle> {
  const token = generateToken();
  const { cmd, args, cwd } = resolveDaemonCommand(opts);
  const timeoutMs = opts.handshakeTimeoutMs ?? DEFAULT_HANDSHAKE_TIMEOUT_MS;

  const child = spawn(cmd, args, {
    cwd,
    env: { ...process.env, ...opts.env, COMPANIOND_TOKEN: token },
    stdio: ["pipe", "pipe", "pipe"],
  });

  return new Promise<DaemonHandle>((resolve, reject) => {
    let settled = false;
    let stdoutBuf = "";
    let gotAddress = false;

    const timer = setTimeout(() => {
      if (settled) return;
      settled = true;
      cleanup();
      reject(new Error(`companiond did not print an address within ${timeoutMs}ms`));
      try {
        child.kill("SIGKILL");
      } catch {
        /* already gone */
      }
    }, timeoutMs);

    function cleanup() {
      clearTimeout(timer);
    }

    child.stdout.on("data", (chunk: Buffer) => {
      if (gotAddress) return; // remaining stdout is ignored per contract
      stdoutBuf += chunk.toString("utf8");
      const nl = stdoutBuf.indexOf("\n");
      if (nl === -1) return;
      const line = stdoutBuf.slice(0, nl).trim();
      gotAddress = true;
      if (settled) return;
      settled = true;
      cleanup();
      if (!/^[^\s]+:\d+$/.test(line)) {
        reject(new Error(`companiond printed an unparseable address line: ${JSON.stringify(line)}`));
        return;
      }
      resolve({ child, baseUrl: `http://${line}`, token });
    });

    let stderrBuf = "";
    child.stderr.on("data", (chunk: Buffer) => {
      stderrBuf += chunk.toString("utf8");
      let idx;
      while ((idx = stderrBuf.indexOf("\n")) !== -1) {
        const line = stderrBuf.slice(0, idx);
        stderrBuf = stderrBuf.slice(idx + 1);
        opts.onLog?.(line);
      }
    });

    child.once("error", (err) => {
      if (settled) return;
      settled = true;
      cleanup();
      reject(err);
    });

    child.once("exit", (code, signal) => {
      if (settled) return;
      settled = true;
      cleanup();
      reject(new Error(`companiond exited before handshake (code=${code} signal=${signal})`));
    });
  });
}

/** Polls GET /healthz (no auth required on that route) until it 200s. */
export async function waitForHealthy(
  baseUrl: string,
  opts: { timeoutMs?: number; intervalMs?: number } = {}
): Promise<void> {
  const timeoutMs = opts.timeoutMs ?? 15_000;
  const intervalMs = opts.intervalMs ?? 150;
  const deadline = Date.now() + timeoutMs;
  let lastErr: unknown;
  while (Date.now() < deadline) {
    try {
      const res = await fetch(`${baseUrl}/healthz`);
      if (res.ok) return;
      lastErr = new Error(`/healthz returned ${res.status}`);
    } catch (err) {
      lastErr = err;
    }
    await new Promise((r) => setTimeout(r, intervalMs));
  }
  throw new Error(
    `companiond never became healthy within ${timeoutMs}ms: ${String(lastErr)}`
  );
}

/**
 * Kills the child per the stdin-close contract: close stdin first (the
 * primary signal companiond watches for), then escalate to SIGTERM and
 * finally SIGKILL if it hasn't exited within the grace windows. Resolves
 * once the process has actually exited.
 */
export function killDaemon(
  child: ChildProcessWithoutNullStreams,
  opts: { termGraceMs?: number; killGraceMs?: number } = {}
): Promise<void> {
  const termGraceMs = opts.termGraceMs ?? 3000;
  const killGraceMs = opts.killGraceMs ?? 2000;

  return new Promise((resolve) => {
    if (child.exitCode !== null || child.signalCode !== null) {
      resolve();
      return;
    }

    let done = false;
    child.once("exit", () => {
      if (done) return;
      done = true;
      clearTimeout(termTimer);
      clearTimeout(killTimer);
      resolve();
    });

    try {
      child.stdin.end();
    } catch {
      /* already closed */
    }

    const termTimer = setTimeout(() => {
      if (done) return;
      try {
        child.kill("SIGTERM");
      } catch {
        /* ignore */
      }
    }, termGraceMs);

    const killTimer = setTimeout(() => {
      if (done) return;
      try {
        child.kill("SIGKILL");
      } catch {
        /* ignore */
      }
    }, termGraceMs + killGraceMs);
  });
}
