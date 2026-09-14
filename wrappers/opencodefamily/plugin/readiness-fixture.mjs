// SPDX-License-Identifier: MIT
import fs from "node:fs";
import { syncBuiltinESMExports } from "node:module";
import { EventEmitter } from "node:events";
import { parentPort, workerData } from "node:worker_threads";

// Model a filesystem subscription which has not become active yet. No event
// or delay is manufactured to make the readiness implementation progress.
fs.watch = () => {
  const watcher = new EventEmitter();
  watcher.close = () => queueMicrotask(() => watcher.emit("close"));
  return watcher;
};
const actualLstat = fs.promises.lstat;
fs.promises.lstat = async (...args) => {
  try { return await actualLstat(...args); }
  catch (error) {
    if (error.code === "ENOENT") parentPort.postMessage({ type: "absent" });
    throw error;
  }
};
syncBuiltinESMExports();
const { waitForEndpoint } = await import("./readiness.mjs");
const lifetime = new AbortController();
const abort = () => lifetime.abort(new Error("worker disposed"));
parentPort.on("message", abort);
try {
  const endpoint = await waitForEndpoint(workerData.directory, lifetime.signal);
  parentPort.postMessage({ type: "result", endpoint });
} catch (error) {
  parentPort.postMessage({ type: "result", error: error.message });
} finally {
  parentPort.off("message", abort);
  parentPort.close();
}
