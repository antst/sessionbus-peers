// SPDX-License-Identifier: MIT
import { watch } from "node:fs";
import { lstat } from "node:fs/promises";
import path from "node:path";

// The launch directory already exists. Watch before checking to cover native
// constructor-before-TUI startup, without a timer or connection retry. Only the
// TUI listener creates the empty marker; the launcher removes owned resources.
export async function waitForEndpoint(directory, signal) {
  if (!signal || signal.aborted) throw signal?.reason || new Error("plugin lifetime ended");
  const marker = path.join(directory, "actions.ready");
  const watcher = watch(directory);
  const closed = new Promise((resolve) => watcher.once("close", resolve));
  let checking = Promise.resolve();
  let active = false;
  let again = false;
  let settled = false;
  let abort;
  try {
    await new Promise((resolve, reject) => {
      const finish = (error) => {
        if (settled) return;
        settled = true;
        if (error) reject(error); else resolve();
      };
      const check = () => {
        if (settled) return;
        if (active) { again = true; return; }
        active = true;
        checking = (async () => {
          do {
            again = false;
            try {
              const info = await lstat(marker);
              if (!info.isFile() || info.size !== 0) throw new Error("invalid Sessionbus endpoint readiness marker");
              finish();
            } catch (error) {
              if (error.code !== "ENOENT") finish(error);
            }
          } while (again && !settled);
          active = false;
        })();
      };
      abort = () => finish(signal.reason || new Error("plugin lifetime ended"));
      signal.addEventListener("abort", abort, { once: true });
      watcher.on("error", finish);
      watcher.on("change", check);
      if (signal.aborted) abort(); else check();
    });
    return path.join(directory, "actions.sock");
  } finally {
    settled = true;
    signal.removeEventListener("abort", abort);
    watcher.close();
    await closed;
    await checking;
  }
}
