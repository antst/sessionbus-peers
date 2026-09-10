// SPDX-License-Identifier: MIT

import { createHash } from "node:crypto";
import net from "node:net";
import kit from "@sessionbus/kit";
import { tool } from "@opencode-ai/plugin";

const { ACTIONS, connectPeer, validate } = kit;
const LIMIT = 1024 * 1024;
const CONNECTION_ENV = ["SESSIONBUS_SOCKET", "SESSIONBUS_LOCAL_KEY"];
const PROCESS_ENV = Object.freeze(Object.fromEntries(Object.entries(process.env).filter(([name]) => name.startsWith("SESSIONBUS_"))));
for (const name of Object.keys(PROCESS_ENV)) delete process.env[name];

function text(value, maximum = LIMIT) {
  return typeof value === "string" && value.length > 0 && Buffer.byteLength(value) <= maximum && !value.includes("\0");
}

function sessionID(value) {
  return typeof value === "string" && value.startsWith("ses") && [...value].length <= 128 && /^[\p{L}\p{M}\p{N}\p{P}\p{S}]+$/u.test(value);
}

function title(value, id) {
  if (typeof value !== "string") throw new Error("OpenCode session title is malformed");
  return value || id;
}

function eventInfo(event) {
  const properties = event?.properties || {};
  return properties.info || properties.session || properties;
}

function eventID(event) {
  const properties = event?.properties || {};
  return properties.info?.id || properties.session?.id || properties.sessionID || "";
}

function render(request) {
  if (!text(request?.from?.session_id, 256) || !text(request?.from?.product, 256)) throw new Error("structured message sender is incomplete");
  const clean = (value) => String(value).replace(/["<>\r\n]/gu, "");
  const name = request.from.name || request.from.session_id;
  const escaped = { "<": "\\u003c", ">": "\\u003e", "&": "\\u0026", "\u2028": "\\u2028", "\u2029": "\\u2029" };
  const metadata = JSON.stringify({ fromProduct: request.from.product, messageId: request.message_id, groups: request.from.groups || [] }).replace(/[<>&\u2028\u2029]/gu, (character) => escaped[character]);
  const body = String(request.body || "").replace(/<\/cross-session-message/giu, "<\\/cross-session-message");
  return `<cross-session-message from="${clean(name)}" from-session="${clean(request.from.session_id)}">\n[sessionbus-metadata: ${metadata}]\n${body}\n</cross-session-message>`;
}

function messageID(value) {
  return `msg_${createHash("sha256").update(String(value)).digest("hex").slice(0, 32)}`;
}

function exact(result, status, message) {
  if (!result || result.error != null || result.response?.status !== status) throw new Error(message);
  return result.data;
}

function connectionEnvironment(saved) {
  return Object.fromEntries(CONNECTION_ENV.map((name) => [name, saved[name]]));
}

function privateAction(path, action, argumentsValue, signal) {
  if (signal?.aborted) return Promise.reject(signal.reason || new Error("cancelled"));
  return new Promise((resolve, reject) => {
    const socket = net.createConnection(path);
    let body = Buffer.alloc(0), finished = false;
    const done = (error, value) => {
      if (finished) return;
      finished = true;
      signal?.removeEventListener("abort", abort);
      socket.destroy();
      error ? reject(error) : resolve(value);
    };
    const abort = () => done(signal.reason || new Error("cancelled"));
    signal?.addEventListener("abort", abort, { once: true });
    socket.on("connect", () => socket.write(`${JSON.stringify({ action, arguments: argumentsValue || {} })}\n`));
    socket.on("data", (chunk) => {
      body = Buffer.concat([body, chunk]);
      if (body.length > LIMIT) return done(new Error("sessionbus lane response exceeds 1 MiB"));
    });
    socket.on("end", () => {
      const newline = body.indexOf(10);
      if (newline < 0) return done(new Error("sessionbus lane returned an empty response"));
      if (newline !== body.length - 1) return done(new Error("sessionbus lane returned a trailing frame"));
      let response;
      try { response = JSON.parse(body.subarray(0, newline)); } catch { return done(new Error("sessionbus lane returned malformed JSON")); }
      if (!response || typeof response !== "object" || Array.isArray(response)) return done(new Error("sessionbus lane returned malformed JSON"));
      const hasError = response.error != null, hasResult = Object.hasOwn(response, "result");
      if (hasError === hasResult) return done(new Error("sessionbus lane response must contain exactly one result or error"));
      if (hasError) {
        const error = new Error(response.error.message || "sessionbus lane call failed");
        error.code = response.error.code;
        error.data = response.error.data;
        return done(error);
      }
      done(null, response.result);
    });
    socket.on("error", (error) => done(error));
  });
}

export function createPlugin(dependencies = {}) {
  const defineTool = dependencies.tool || tool;
  const makePeer = dependencies.connectPeer || connectPeer;
  const callLane = dependencies.privateAction || privateAction;
  const onExit = dependencies.onExit || ((callback) => process.once("beforeExit", callback));
  const report = dependencies.reportError || ((error) => console.error(`sessionbus: OpenCode peer delivery: ${error?.message || error}`));
  return async function sessionbusOpenCode({ client, directory }) {
    const laneSocket = PROCESS_ENV.SESSIONBUS_LANE_SOCKET || "";
    if (laneSocket && !text(laneSocket, 4096)) throw new Error("SESSIONBUS_LANE_SOCKET is invalid");
    const groups = JSON.parse(PROCESS_ENV.SESSIONBUS_GROUPS || "[]");
    if (!Array.isArray(groups) || groups.some((group) => !text(group, 128))) throw new Error("SESSIONBUS_GROUPS must be a JSON array of names");
    const requestedName = PROCESS_ENV.SESSIONBUS_SESSION_NAME || "";
    const sessions = new Map();
    const submit = async (current, signal) => {
      const item = current.queue.shift();
      if (!item || sessions.get(item.sessionID) !== current) return false;
      const statuses = exact(await client.session.status({ query: { directory } }, { signal }), 200, "OpenCode legacy status was not confirmed");
      const status = statuses && typeof statuses === "object" && !Array.isArray(statuses) ? Object.hasOwn(statuses, item.sessionID) ? statuses[item.sessionID]?.type || "malformed" : "" : "malformed";
      if (status === "busy" || status === "retry") {
        current.queue.unshift(item);
        return false;
      }
      if (status && status !== "idle") throw new Error("OpenCode legacy status is malformed");
      const info = exact(await client.session.get({ path: { id: item.sessionID }, query: { directory } }, { signal }), 200, "OpenCode legacy session was not confirmed");
      if (sessions.get(item.sessionID) !== current) return false;
      if (info?.id !== item.sessionID) throw new Error("OpenCode returned the wrong legacy session");
      const model = info.model && { providerID: info.model.providerID, modelID: info.model.id };
      const body = { messageID: item.id, ...(info.agent ? { agent: info.agent } : {}), ...(model ? { model } : {}), ...(info.model?.variant && info.model.variant !== "default" ? { variant: info.model.variant } : {}), parts: [{ type: "text", text: item.text }] };
      const response = await Promise.resolve().then(() => client.session.promptAsync({ path: { id: item.sessionID }, query: { directory }, body }, { signal })).catch(() => Promise.reject(new Error("OpenCode native delivery outcome is unknown")));
      exact(response, 204, "OpenCode legacy prompt lacked exact native acceptance");
      return true;
    };
    const drain = (current, signal) => current.active || (current.active = submit(current, signal).finally(() => current.active = undefined));
    const deliver = async (signal, request, identity) => {
      if (signal.aborted) return { disposition: "rejected", reason: "closing" };
      const current = sessions.get(identity.session_id);
      if (!current) return { disposition: "rejected", reason: "closing" };
      const item = { sessionID: identity.session_id, id: messageID(request.message_id), text: render(request) };
      current.queue.push(item);
      if (current.active || current.queue[0] !== item) return { disposition: "queued_for_next_turn" };
      await drain(current, signal);
      return { disposition: "queued_for_next_turn" };
    };
    const publish = async (event) => {
      const info = eventInfo(event), id = eventID(event);
      if (!sessionID(id) || !Object.hasOwn(info, "title")) return;
      const nativeName = title(info.title, id);
      const nativeIdentity = { product: "opencode", session_id: id, name: nativeName, groups: [...groups], info: { cwd: info.directory || directory } };
      const requestedIdentity = event.type === "session.created" && requestedName ? { ...nativeIdentity, name: requestedName } : nativeIdentity;
      if (!validate("SessionHelloRequest", { protocol: 1, ...nativeIdentity }) || !validate("SessionHelloRequest", { protocol: 1, ...requestedIdentity })) throw new Error("OpenCode peer identity is outside the Sessionbus grammar");
      let current = sessions.get(id);
      if (!current && !laneSocket && PROCESS_ENV.SESSIONBUS_SOCKET) {
        current = { identity: nativeIdentity, queue: [] };
        current.peer = makePeer(nativeIdentity, deliver, connectionEnvironment(PROCESS_ENV));
        sessions.set(id, current);
      }
      if (event.type === "session.created" && requestedName && nativeName !== requestedName) {
        const updated = await client.session.update({ path: { id }, query: { directory }, body: { title: requestedName } });
        if (updated?.error != null || updated?.response?.status !== 200 || updated.data?.id !== id || updated.data?.title !== requestedName) throw new Error("OpenCode did not confirm the requested session title");
      }
      if (current && (current.identity.name !== requestedIdentity.name || current.identity.info.cwd !== requestedIdentity.info.cwd)) {
        try { await current.peer.rehello(undefined, requestedIdentity.name, requestedIdentity.info); }
        catch (error) { if (sessions.get(id) === current) throw error; return; }
        if (sessions.get(id) === current) current.identity = requestedIdentity;
      }
    };
    const caller = (id) => laneSocket ? { action: (action, args, signal) => callLane(laneSocket, action, args, signal) } : sessions.get(id)?.peer;
    const hooks = {
      event: async ({ event }) => {
        if (event?.type === "session.created" || event?.type === "session.updated") await publish(event);
        if (event?.type === "session.status" && event?.properties?.status?.type === "idle") {
          const current = sessions.get(eventID(event));
          if (current && (current.active || current.queue.length)) {
            await Promise.resolve(current.active).then((accepted) => accepted || drain(current)).catch((error) => Promise.resolve().then(() => report(error)).catch(() => undefined));
          }
        }
        if (event?.type === "session.deleted") {
          const current = sessions.get(eventID(event));
          current?.peer.shutdown();
          sessions.delete(eventID(event));
        }
      },
      "shell.env": async (input, output) => {
        if (!sessionID(input.sessionID)) throw new Error("shell context omitted the exact OpenCode session identity");
        output.env.SESSIONBUS_SESSION_ID = input.sessionID;
      },
      tool: {
        sessionbus: defineTool({
          description: "List and message sessionbus peers or control product lanes. List returns self_info for the bound originating caller even when filters exclude its row; identify self by self_info.session_id, never names or list order. Older daemons may omit self_info.",
          args: {
            action: defineTool.schema.enum(ACTIONS),
            arguments: defineTool.schema.record(defineTool.schema.string(), defineTool.schema.any()).default({}),
          },
          execute: async ({ action, arguments: args = {} }, context) => {
            if (!sessionID(context.sessionID)) throw new Error("tool context omitted the exact OpenCode session identity");
            const current = caller(context.sessionID);
            if (!current) throw new Error("sessionbus has no live OpenCode peer for this session");
            await current.ready;
            return JSON.stringify(await (current.caller || current).action(action, args, context.abort));
          },
        }),
      },
    };
    onExit(() => { for (const current of sessions.values()) current.peer.shutdown(); sessions.clear(); });
    return hooks;
  };
}

export default createPlugin();
