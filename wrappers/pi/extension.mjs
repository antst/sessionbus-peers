// SPDX-License-Identifier: MIT
import { lstatSync, realpathSync } from "node:fs";
import path from "node:path";

import {
  BridgeCallError,
  BridgeClosedError,
  BridgeProtocolError,
  connectBridge,
} from "../pifamily/extension/bridge.mjs";
import { appendNative, describeNative } from "./native.mjs";

export const launchEnvironmentName = "SESSIONBUS_PI_LAUNCH";
export const toolName = "sessionbus";

const processExtensionKey = Symbol.for("sessionbus.pi.managed-extension.v1");

const actions = Object.freeze([
  "list", "send", "spawn", "describe", "run", "start", "wait", "status",
  "interrupt", "close", "forget", "ack",
]);
const topologyMode = Object.freeze({ lane: "rpc", interactive: "tui" });
const drainWitnesses = Object.freeze(["before_agent_start", "agent_settled"]);
const sessionEndReasons = Object.freeze(["quit", "reload", "new", "resume", "fork"]);

const toolDescription = `Call Sessionbus using this exact Pi session identity.
Use list to discover peers and self_info, send to message peers, describe before
spawning a product lane, and run/start/status/wait/ack/interrupt/close/forget to
own a lane through its full lifecycle. Arguments must have the exact public
shape for the selected action. A successful send confirms only the recipient's
published delivery disposition; queued_for_next_turn is not model consumption.
A completion pointer is an ordinary peer message, not the lane result.`;

function object(value) {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}

function exactKeys(value, keys, what) {
  if (!object(value)) throw new Error(`Pi ${what} is not an object`);
  const actual = Object.keys(value);
  if (actual.length !== keys.length || keys.some((key) => !Object.hasOwn(value, key))) {
    throw new Error(`Pi ${what} fields are invalid`);
  }
  return value;
}

function boundedString(value, limit, what, { empty = false } = {}) {
  if (typeof value !== "string" || (!empty && value.length === 0) ||
      Buffer.byteLength(value) > limit || /\0/u.test(value)) {
    throw new Error(`Pi ${what} is invalid`);
  }
  return value;
}

function nativeID(value) {
  return boundedString(value, 256, "native session identity");
}

function toolCallID(value) {
  return boundedString(value, 256, "native tool call identity");
}

function abortReason(signal) {
  if (signal?.reason instanceof Error) return signal.reason;
  const error = new Error("The operation was aborted");
  error.name = "AbortError";
  return error;
}

function throwIfAborted(signal) {
  if (signal?.aborted) throw abortReason(signal);
}

function combinedSignal(...signals) {
  const present = signals.filter(Boolean);
  if (present.length === 0) return undefined;
  if (present.length === 1) return present[0];
  return AbortSignal.any(present);
}

function validatePhysicalLaunch(launch, fs = { lstatSync, realpathSync }) {
  const directory = fs.lstatSync(launch.directory);
  if (!directory.isDirectory() || directory.isSymbolicLink() ||
      (directory.mode & 0o777) !== 0o700 || directory.uid !== process.getuid() ||
      fs.realpathSync(launch.directory) !== launch.directory) {
    throw new Error("Pi managed launch directory is not private, physical, and owned");
  }
  const socket = fs.lstatSync(launch.socket);
  if (!socket.isSocket() || socket.isSymbolicLink() || socket.uid !== process.getuid() ||
      path.dirname(launch.socket) !== launch.directory) {
    throw new Error("Pi managed bridge socket is not physical and owned");
  }
}

// Capture and scrub before native tools or nested processes can inherit the
// owner binding. A missing binding remains inert until Pi tries to load this
// explicitly managed extension, where it fails closed.
export function captureLaunch(environment = process.env, parent = process.ppid, fs = { lstatSync, realpathSync }) {
  const raw = environment[launchEnvironmentName];
  delete environment[launchEnvironmentName];
  if (raw === undefined) return null;
  if (typeof raw !== "string" || Buffer.byteLength(raw) > 64 << 10) {
    throw new Error("Pi managed launch metadata is invalid");
  }
  let launch;
  try {
    launch = JSON.parse(raw);
  } catch {
    throw new Error("Pi managed launch metadata is invalid");
  }
  exactKeys(launch, ["directory", "owner_pid", "socket", "topology"], "managed launch metadata");
  if (!Number.isSafeInteger(launch.owner_pid) || launch.owner_pid <= 1 || launch.owner_pid !== parent ||
      !Object.hasOwn(topologyMode, launch.topology) ||
      !path.isAbsolute(boundedString(launch.directory, 4096, "managed launch directory")) ||
      !path.isAbsolute(boundedString(launch.socket, 4096, "managed bridge socket"))) {
    throw new Error("Pi managed launch metadata is invalid");
  }
  validatePhysicalLaunch(launch, fs);
  return Object.freeze({ ...launch });
}

function validateEcho(result, sessionID, extra = {}) {
  const keys = ["session_id", ...Object.keys(extra)];
  exactKeys(result, keys, "bridge response");
  if (result.session_id !== sessionID) throw new Error("Pi bridge changed native session identity");
  for (const [key, expected] of Object.entries(extra)) {
    if (result[key] !== expected) throw new Error(`Pi bridge changed ${key}`);
  }
  return result;
}

function toolParameters() {
  return {
    type: "object",
    additionalProperties: false,
    required: ["action", "arguments"],
    properties: {
      action: { type: "string", enum: [...actions] },
      arguments: { type: "object" },
    },
  };
}

export function createPiExtension({
  launch,
  connect = connectBridge,
  describe = describeNative,
  append = appendNative,
} = {}) {
  const lifetime = new AbortController();
  let bridgePromise;
  let bridge;
  let bridgeClosing = false;
  let current;
  let failure;
  let settling = false;

  function stop(ctx, error) {
    if (failure) return;
    failure = error instanceof Error ? error : new Error(String(error));
    if (!lifetime.signal.aborted) lifetime.abort(failure);
    try { ctx?.abort?.(); } catch {}
    try { ctx?.shutdown?.(); } catch {}
    if (bridge) void bridge.close().catch(() => {});
  }

  function live(ctx = current?.ctx, pi = current?.pi) {
    if (!current || !ctx || !pi) throw new Error("Pi native session is not bound");
    const info = describe(pi, ctx);
    if (info.session_id !== current.session_id) {
      const error = new Error("Pi native session identity changed without a lifecycle event");
      stop(ctx, error);
      throw error;
    }
    return { record: current, info };
  }

  async function nativeRequest({ method, params, signal }) {
    throwIfAborted(signal);
    const { record } = live();
    switch (method) {
      case "native.describe": {
        exactKeys(params, ["session_id"], "native.describe parameters");
        if (params.session_id !== record.session_id) {
          throw new BridgeCallError("stale_session", "Pi native session identity does not match");
        }
        const { info } = live();
        return info;
      }
      case "native.append": {
        if (launch.topology !== "interactive") {
          throw new BridgeCallError("method_not_found", "Pi native.append is unavailable in lane topology");
        }
        exactKeys(params, ["session_id", "message_id", "body"], "native.append parameters");
        if (params.session_id !== record.session_id) {
          throw new BridgeCallError("stale_session", "Pi native session identity does not match");
        }
        throwIfAborted(signal);
        const result = append(record.pi, record.ctx, params);
        throwIfAborted(signal);
        if (result.accepted === false) {
          exactKeys(result, ["accepted", "reason"], "native.append result");
          if (result.reason !== "busy") throw new Error("Pi native append rejection is invalid");
          return { session_id: record.session_id, message_id: params.message_id, accepted: false, reason: "busy" };
        }
        exactKeys(result, ["accepted", "entry_id"], "native.append result");
        if (result.accepted !== true) throw new Error("Pi native append result is invalid");
        boundedString(result.entry_id, 256, "native entry identity");
        return { session_id: record.session_id, message_id: params.message_id, accepted: true, entry_id: result.entry_id };
      }
      default:
        throw new BridgeCallError("method_not_found", "Pi bridge method is unavailable");
    }
  }

  async function connection() {
    if (!launch) throw new Error("Pi managed launch metadata is missing");
    if (failure) throw failure;
    if (!bridgePromise) {
      bridgePromise = Promise.resolve(connect(launch.socket, {
        role: "native",
        handler: nativeRequest,
        signal: lifetime.signal,
      })).then((value) => {
        bridge = value;
        void value.done.then(() => {
          if (!bridgeClosing && !lifetime.signal.aborted) {
            stop(current?.ctx, new BridgeClosedError("Pi managed owner connection ended"));
          }
        });
        return value;
      });
      bridgePromise.catch((error) => stop(current?.ctx, error));
    }
    return bridgePromise;
  }

  async function hostCall(ctx, method, params, signal = undefined) {
    try {
      const owner = await connection();
      return await owner.call(method, params, { signal: combinedSignal(lifetime.signal, current?.controller.signal, signal) });
    } catch (error) {
      stop(ctx, error);
      throw error;
    }
  }

  function requireMode(ctx) {
    if (!launch || ctx.mode !== topologyMode[launch.topology]) {
      throw new Error("Pi native mode does not match its managed topology");
    }
  }

  async function ready(pi, ctx) {
    requireMode(ctx);
    if (current) throw new Error("Pi native session started before its predecessor ended");
    const info = describe(pi, ctx);
    current = { pi, ctx, session_id: nativeID(info.session_id), controller: new AbortController() };
    settling = false;
    const result = await hostCall(ctx, "owner.ready", {
      topology: launch.topology,
      directory: launch.directory,
      session_id: current.session_id,
      name: boundedString(info.name, 4096, "native name", { empty: true }),
    });
    validateEcho(result, current.session_id);
  }

  async function reportReady(pi, ctx) {
    const { record, info } = live(ctx, pi);
    const result = await hostCall(ctx, "owner.ready", {
      topology: launch.topology,
      directory: launch.directory,
      session_id: record.session_id,
      name: boundedString(info.name, 4096, "native name", { empty: true }),
    });
    validateEcho(result, record.session_id);
  }

  async function drain(ctx, witness) {
    if (launch.topology !== "interactive") return;
    if (!drainWitnesses.includes(witness)) throw new Error("Pi native drain witness is invalid");
    const { record } = live(ctx);
    const result = await hostCall(ctx, "owner.drain", { session_id: record.session_id, witness });
    exactKeys(result, ["session_id", "drained"], "owner.drain response");
    if (result.session_id !== record.session_id || !Number.isSafeInteger(result.drained) || result.drained < 0 || result.drained > 256) {
      throw new Error("Pi owner.drain response is invalid");
    }
  }

  async function laneWitness(ctx, method, params) {
    if (launch.topology !== "lane") return;
    const { record } = live(ctx);
    const result = await hostCall(ctx, method, { session_id: record.session_id, ...params });
    validateEcho(result, record.session_id);
  }

  return function sessionbusPiExtension(pi) {
    if (!launch) throw new Error("Pi managed launch metadata is missing");

    pi.registerTool({
      name: toolName,
      label: "Sessionbus",
      description: toolDescription,
      parameters: toolParameters(),
      async execute(callID, params, signal, _onUpdate, ctx) {
        const { record } = live(ctx, pi);
        toolCallID(callID);
        exactKeys(params, ["action", "arguments"], "Sessionbus tool arguments");
        if (!actions.includes(params.action) || !object(params.arguments)) {
          throw new Error("Pi Sessionbus tool arguments are invalid");
        }
        let response;
        try {
          response = await (await connection()).call("tool.call", {
            session_id: record.session_id,
            call_id: callID,
            action: params.action,
            arguments: params.arguments,
          }, { signal: combinedSignal(lifetime.signal, record.controller.signal, signal) });
        } catch (error) {
          if (!(error instanceof BridgeCallError)) stop(ctx, error);
          throw error;
        }
        exactKeys(response, ["session_id", "call_id", "result"], "tool.call response");
        if (response.session_id !== record.session_id || response.call_id !== callID || !object(response.result)) {
          throw new Error("Pi tool.call response is invalid");
        }
        const text = JSON.stringify(response.result);
        if (typeof text !== "string" || Buffer.byteLength(text) > 1 << 20) {
          throw new Error("Pi Sessionbus result exceeds its native bound");
        }
        return { content: [{ type: "text", text }], details: response };
      },
    });

    pi.on("session_start", async (_event, ctx) => {
      try { await ready(pi, ctx); } catch (error) { stop(ctx, error); }
    });

    pi.on("session_info_changed", async (_event, ctx) => {
      try { await reportReady(pi, ctx); } catch (error) { stop(ctx, error); }
    });

    pi.on("input", (event, ctx) => {
      if (launch.topology !== "lane") return { action: "continue" };
      if (event.source === "rpc") settling = false;
      return (async () => {
        try {
          if (event.images?.length) throw new Error("Pi lane input does not support images");
          await laneWitness(ctx, "run.input", {
            source: event.source,
            text: boundedString(event.text, 1 << 20, "native input", { empty: true }),
            settling,
          });
          return { action: "continue" };
        } catch (error) {
          stop(ctx, error);
          return { action: "handled" };
        }
      })();
    });

    pi.on("before_agent_start", (_event, ctx) => {
      if (launch.topology === "interactive") {
        return drain(ctx, "before_agent_start").catch((error) => stop(ctx, error));
      }
      return laneWitness(ctx, "run.preflight", {
        prompt: boundedString(_event.prompt, 1 << 20, "native preflight prompt", { empty: true }),
        settling,
      }).catch((error) => stop(ctx, error));
    });

    pi.on("agent_start", (_event, ctx) => {
      if (launch.topology !== "lane") return;
      return laneWitness(ctx, "run.start", { settling }).catch((error) => stop(ctx, error));
    });

    pi.on("agent_settled", (_event, ctx) => {
      // This prefix runs in the first CLI extension before any await or global
      // settled handler. Pi has already cleared its active flag.
      settling = true;
      if (launch.topology === "interactive") {
        return drain(ctx, "agent_settled").catch((error) => stop(ctx, error));
      }
      return laneWitness(ctx, "run.settling", {}).catch((error) => stop(ctx, error));
    });

    pi.on("session_shutdown", async (event, ctx) => {
      const record = current;
      if (!record) return;
      try {
        if (!sessionEndReasons.includes(event.reason)) throw new Error("Pi native shutdown reason is invalid");
        live(ctx, pi);
        const result = await hostCall(ctx, "session_end", {
          topology: launch.topology,
          session_id: record.session_id,
          reason: event.reason,
        });
        validateEcho(result, record.session_id);
      } catch (error) {
        stop(ctx, error);
      } finally {
        if (!record.controller.signal.aborted) {
          record.controller.abort(new BridgeClosedError("Pi native session ended"));
        }
        if (current === record) current = undefined;
        if (event.reason === "quit") {
          bridgeClosing = true;
          if (bridge) await bridge.close().catch(() => {});
          if (!lifetime.signal.aborted) lifetime.abort(new BridgeClosedError("Pi native session ended"));
        }
      }
    });
  };
}

let processExtension = globalThis[processExtensionKey];
if (processExtension === undefined) {
  const capturedLaunch = captureLaunch();
  processExtension = createPiExtension({ launch: capturedLaunch });
  // Ordinary, unmanaged imports remain inert and do not reserve this
  // process-global slot. A managed reload occurs after the descriptor has
  // been scrubbed, so it must retain the original factory and connection.
  if (capturedLaunch !== null) {
    Object.defineProperty(globalThis, processExtensionKey, {
      configurable: false,
      enumerable: false,
      writable: false,
      value: processExtension,
    });
  }
}
export default processExtension;
