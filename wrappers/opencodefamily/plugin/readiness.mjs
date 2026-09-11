// SPDX-License-Identifier: MIT
import { lstat, open } from "node:fs/promises";
import path from "node:path";

const channelName = (directory) => "sessionbus:endpoint:" + directory;

// Called only by the owning TUI after its endpoint listens. The file remains
// the readiness authority; the process-local broadcast is only a wake-up.
export async function publishEndpoint(directory) {
  const marker = await open(path.join(directory, "actions.ready"), "wx", 0o600);
  await marker.close();
  const channel = new BroadcastChannel(channelName(directory));
  try { channel.postMessage(null); }
  finally { channel.close(); }
}

// TUI and server Workers share the native process. BroadcastChannel subscribes
// synchronously, unlike Darwin's asynchronous filesystem watcher. Subscribe
// before checking: a prior publication leaves the marker; a later one wakes us.
export async function waitForEndpoint(directory, signal) {
  if (!signal || signal.aborted) throw signal?.reason || new Error("plugin lifetime ended");
  const marker = path.join(directory, "actions.ready");
  const channel = new BroadcastChannel(channelName(directory));
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
      channel.onmessageerror = () => finish(new Error("Sessionbus readiness wake could not be decoded"));
      channel.onmessage = check;
      if (signal.aborted) abort(); else check();
    });
    return path.join(directory, "actions.sock");
  } finally {
    settled = true;
    signal.removeEventListener("abort", abort);
    channel.close();
    await checking;
  }
}
