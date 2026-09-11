import net from "node:net";

export const BRIDGE_PROTOCOL_VERSION = 1;
const MAX_BRIDGE_REQUEST_ID = Number.MAX_SAFE_INTEGER;

export const DEFAULT_BRIDGE_LIMITS = Object.freeze({
  maxFrameBytes: 1 << 20,
  maxPendingCalls: 256,
  maxActiveCalls: 256,
  maxPendingWrites: 256,
  maxRetainedBytes: 32 << 20,
});

export class BridgeClosedError extends Error {
  constructor(message = "Pi-family bridge is closed") {
    super(message);
    this.name = "BridgeClosedError";
  }
}

export class BridgeBusyError extends Error {
  constructor(message = "Pi-family bridge is busy") {
    super(message);
    this.name = "BridgeBusyError";
  }
}

export class BridgeProtocolError extends Error {
  constructor(message) {
    super(message);
    this.name = "BridgeProtocolError";
  }
}

export class BridgeCallError extends Error {
  constructor(code, message) {
    super(message);
    this.name = "BridgeCallError";
    this.code = code;
  }
}

function normalizedLimits(input = {}) {
  for (const key of Object.keys(input)) {
    if (!Object.hasOwn(DEFAULT_BRIDGE_LIMITS, key)) {
      throw new TypeError(`Pi-family bridge limit ${key} is unknown`);
    }
  }
  const limits = { ...DEFAULT_BRIDGE_LIMITS, ...input };
  for (const key of Object.keys(DEFAULT_BRIDGE_LIMITS)) {
    if (!Number.isSafeInteger(limits[key])) {
      throw new TypeError(`Pi-family bridge limit ${key} is invalid`);
    }
  }
  if (
    limits.maxFrameBytes < 256 ||
    limits.maxPendingCalls < 1 ||
    limits.maxActiveCalls < 1 ||
    limits.maxPendingWrites < 1 ||
    limits.maxRetainedBytes < limits.maxFrameBytes ||
    limits.maxFrameBytes > DEFAULT_BRIDGE_LIMITS.maxFrameBytes ||
    limits.maxPendingCalls > DEFAULT_BRIDGE_LIMITS.maxPendingCalls ||
    limits.maxActiveCalls > DEFAULT_BRIDGE_LIMITS.maxActiveCalls ||
    limits.maxPendingWrites > DEFAULT_BRIDGE_LIMITS.maxPendingWrites ||
    limits.maxRetainedBytes > DEFAULT_BRIDGE_LIMITS.maxRetainedBytes
  ) {
    throw new TypeError("Pi-family bridge limits are invalid");
  }
  return limits;
}

function validMethod(method) {
  return (
    typeof method === "string" &&
    method.length > 0 &&
    Buffer.byteLength(method) <= 128 &&
    /^[A-Za-z][A-Za-z0-9_.-]*$/u.test(method)
  );
}

function validErrorCode(code) {
  return (
    typeof code === "string" &&
    code.length > 0 &&
    Buffer.byteLength(code) <= 64 &&
    /^[a-z][a-z0-9_]*$/u.test(code)
  );
}

function errorMessage(error) {
  const message = error instanceof Error ? error.message : String(error);
  if (Buffer.byteLength(message) <= 1024) return message || "Pi-family bridge handler failed";
  let result = "";
  for (const character of message) {
    if (Buffer.byteLength(result + character) > 1024) break;
    result += character;
  }
  return result || "Pi-family bridge handler failed";
}

function callError(error) {
  if (error instanceof BridgeCallError && validErrorCode(error.code)) {
    return { code: error.code, message: errorMessage(error) };
  }
  if (error?.name === "AbortError") {
    return { code: "cancelled", message: "Pi-family bridge call was cancelled" };
  }
  return { code: "handler_error", message: errorMessage(error) };
}

function exactKeys(object, keys) {
  const actual = Object.keys(object);
  if (actual.length !== keys.length || keys.some((key) => !Object.hasOwn(object, key))) {
    throw new BridgeProtocolError("Pi-family bridge frame fields are invalid");
  }
}

function validateObject(value, what) {
  if (value === null || typeof value !== "object" || Array.isArray(value)) {
    throw new BridgeProtocolError(`Pi-family bridge ${what} is not an object`);
  }
}

function abortReason(signal) {
  if (signal?.reason instanceof Error) return signal.reason;
  const error = new Error("The operation was aborted");
  error.name = "AbortError";
  return error;
}

function requestPrefix(role) {
  return role === "host" ? "h:" : "n:";
}

function parseRequestID(id, role) {
  const prefix = requestPrefix(role);
  if (typeof id !== "string" || !id.startsWith(prefix)) {
    throw new BridgeProtocolError(`Pi-family bridge request id ${JSON.stringify(id)} has the wrong owner`);
  }
  const digits = id.slice(prefix.length);
  if (!/^[1-9][0-9]*$/u.test(digits) || digits.length > 16) {
    throw new BridgeProtocolError(`Pi-family bridge request id ${JSON.stringify(id)} is invalid`);
  }
  const sequence = Number(digits);
  if (!Number.isSafeInteger(sequence) || sequence > MAX_BRIDGE_REQUEST_ID) {
    throw new BridgeProtocolError(`Pi-family bridge request id ${JSON.stringify(id)} is invalid`);
  }
  return sequence;
}

function makeDeferred() {
  let resolve;
  let reject;
  const promise = new Promise((res, rej) => {
    resolve = res;
    reject = rej;
  });
  // A connection may close before a caller asks for readiness or completion.
  // Keep that owned rejection from becoming an unhandled process-level event.
  promise.catch(() => {});
  return { promise, resolve, reject };
}

export class PrivateBridge {
  constructor(socket, { role, handler = undefined, limits = undefined } = {}) {
    if (socket === null || typeof socket?.write !== "function" || typeof socket?.destroy !== "function") {
      throw new TypeError("Pi-family bridge socket is invalid");
    }
    if (role !== "host" && role !== "native") {
      throw new TypeError(`Pi-family bridge role ${JSON.stringify(role)} is invalid`);
    }
    this.socket = socket;
    this.role = role;
    this.peerRole = role === "host" ? "native" : "host";
    this.handler = handler;
    this.limits = normalizedLimits(limits);

    this.stopped = false;
    this.intentional = false;
    this.closed = false;
    this.failure = undefined;
    this.readyState = false;
    this.sawHello = false;
    this.nextOutboundID = 0;
    this.nextInboundID = 0;
    this.pending = new Map();
    this.active = new Map();
    this.writes = new Set();
    this.tasks = new Set();
    this.pendingWriteBytes = 0;
    this.retainedBytes = 0;
    this.buffer = Buffer.alloc(0);
    this.decoder = new TextDecoder("utf-8", { fatal: true });

    this.peerReady = makeDeferred();
    this.doneState = makeDeferred();
    this.done = this.doneState.promise;

    this.onData = (chunk) => this.#onData(chunk);
    this.onError = (error) => this.#stop(error instanceof Error ? error : new Error(String(error)), false);
    this.onEnd = () => this.#stop(new BridgeClosedError("Pi-family bridge peer ended the connection"), false);
    this.onClose = () => void this.#onClose();
    socket.on("data", this.onData);
    socket.on("error", this.onError);
    socket.on("end", this.onEnd);
    socket.on("close", this.onClose);

    this.helloWrite = this.#sendFrame({
      version: BRIDGE_PROTOCOL_VERSION,
      type: "hello",
      role,
    });
    this.helloWrite.catch(() => {});
  }

  async ready(signal = undefined) {
    if (signal?.aborted) {
      const reason = abortReason(signal);
      this.#stop(reason, false);
      throw reason;
    }
    await this.#withAbort(Promise.all([this.helloWrite, this.peerReady.promise]), signal, true);
  }

  async call(method, params = null, { signal } = {}) {
    if (signal?.aborted) throw abortReason(signal);
    if (!validMethod(method)) {
      throw new BridgeProtocolError("Pi-family bridge method is invalid");
    }
    if (!this.readyState) {
      throw new BridgeProtocolError("Pi-family bridge is not ready");
    }
    if (this.stopped) throw this.#connectionError();
    if (this.pending.size >= this.limits.maxPendingCalls) throw new BridgeBusyError();
    if (this.nextOutboundID >= MAX_BRIDGE_REQUEST_ID) {
      throw new BridgeProtocolError("Pi-family bridge request id space is exhausted");
    }

    // Normalize and size the complete frame before consuming its sequential
    // request ID. Failed encoding or capacity admission must leave no gap.
    let normalizedParams;
    try {
      normalizedParams = JSON.parse(JSON.stringify(params));
    } catch (error) {
      throw new BridgeProtocolError(`Pi-family bridge parameters are not JSON: ${errorMessage(error)}`);
    }
    const sequence = this.nextOutboundID + 1;
    const id = `${requestPrefix(this.role)}${sequence}`;
    const frame = this.#encode({
      version: BRIDGE_PROTOCOL_VERSION,
      type: "request",
      id,
      method,
      params: normalizedParams,
    });
    const deferred = makeDeferred();
    const pending = { ...deferred, abandoned: false, settled: false };

    let write;
    try {
      write = this.#send(frame);
    } catch (error) {
      if (signal?.aborted) {
        const reason = abortReason(signal);
        this.#stop(reason, false);
        throw reason;
      }
      throw error;
    }
    // node:net cannot dispatch a peer response during the synchronous write
    // call above. Publish correlation ownership before yielding to its callback.
    this.nextOutboundID = sequence;
    this.pending.set(id, pending);
    try {
      await this.#withAbort(write, signal, false);
    } catch (error) {
      if (this.pending.get(id) !== pending) {
        return this.#consumeResponse(await pending.promise);
      }
      this.pending.delete(id);
      if (signal?.aborted) {
        const reason = abortReason(signal);
        this.#stop(reason, false);
        throw reason;
      }
      throw error;
    }

    const cancel = () => {
      if (this.pending.get(id) !== pending || pending.settled) return;
      pending.abandoned = true;
      pending.settled = true;
      const reason = abortReason(signal);
      pending.reject(reason);
      this.#sendDetachedFrame({ version: BRIDGE_PROTOCOL_VERSION, type: "cancel", id });
    };
    signal?.addEventListener("abort", cancel, { once: true });
    if (signal?.aborted) cancel();
    try {
      return this.#consumeResponse(await pending.promise);
    } finally {
      signal?.removeEventListener("abort", cancel);
    }
  }

  stats() {
    return {
      pendingCalls: this.pending.size,
      activeCalls: this.active.size,
      pendingWrites: this.writes.size,
      retainedBytes: this.retainedBytes + this.pendingWriteBytes,
    };
  }

  #consumeResponse(response) {
    if (response?.bridgeResponse !== true) return response;
    this.retainedBytes -= response.bytes;
    if (response.error) throw response.error;
    return response.result;
  }

  async close() {
    this.#stop(undefined, true);
    await this.done;
    if (this.failure) throw this.failure;
  }

  #connectionError() {
    return this.failure ?? new BridgeClosedError();
  }

  #encode(value) {
    let text;
    try {
      text = JSON.stringify(value);
    } catch (error) {
      throw new BridgeProtocolError(`Pi-family bridge frame is not JSON: ${errorMessage(error)}`);
    }
    if (typeof text !== "string") {
      throw new BridgeProtocolError("Pi-family bridge frame is not JSON");
    }
    const body = Buffer.from(`${text}\n`, "utf8");
    if (body.length - 1 > this.limits.maxFrameBytes) {
      throw new BridgeProtocolError(`Pi-family bridge frame exceeds ${this.limits.maxFrameBytes} bytes`);
    }
    return body;
  }

  #sendFrame(value, signal = undefined) {
    return this.#send(this.#encode(value), signal);
  }

  #sendDetachedFrame(value) {
    try {
      this.#sendFrame(value).catch((error) => this.#stop(error, false));
    } catch (error) {
      this.#stop(error, false);
    }
  }

  #send(body, signal = undefined) {
    if (signal?.aborted) throw abortReason(signal);
    if (this.stopped) throw this.#connectionError();
    if (
      this.writes.size >= this.limits.maxPendingWrites ||
      this.retainedBytes + this.pendingWriteBytes > this.limits.maxRetainedBytes - body.length
    ) {
      throw new BridgeBusyError("Pi-family bridge write capacity is exhausted");
    }

    const deferred = makeDeferred();
    const write = { ...deferred, body, settled: false, onAbort: undefined };
    const settle = (error = undefined) => {
      if (write.settled) return;
      write.settled = true;
      this.writes.delete(write);
      this.pendingWriteBytes -= body.length;
      signal?.removeEventListener("abort", write.onAbort);
      if (error) write.reject(error);
      else write.resolve();
    };
    write.settle = settle;
    write.onAbort = () => {
      const reason = abortReason(signal);
      this.#stop(reason, false);
    };
    this.writes.add(write);
    this.pendingWriteBytes += body.length;
    signal?.addEventListener("abort", write.onAbort, { once: true });
    try {
      this.socket.write(body, (error) => {
        settle(error);
        if (error) this.#stop(error, false);
      });
    } catch (error) {
      settle(error);
      this.#stop(error, false);
      throw error;
    }
    if (signal?.aborted) write.onAbort();
    return write.promise;
  }

  #onData(chunk) {
    if (this.stopped) return;
    try {
      const data = Buffer.isBuffer(chunk) ? chunk : Buffer.from(chunk);
      if (this.buffer.length + data.length > this.limits.maxFrameBytes + 1 && !data.includes(0x0a)) {
        throw new BridgeProtocolError("Pi-family bridge frame exceeds its byte limit");
      }
      this.buffer = this.buffer.length === 0 ? Buffer.from(data) : Buffer.concat([this.buffer, data]);
      while (true) {
        const newline = this.buffer.indexOf(0x0a);
        if (newline < 0) {
          if (this.buffer.length > this.limits.maxFrameBytes) {
            throw new BridgeProtocolError("Pi-family bridge frame exceeds its byte limit");
          }
          return;
        }
        const line = this.buffer.subarray(0, newline);
        this.buffer = this.buffer.subarray(newline + 1);
        if (line.length === 0 || line.length > this.limits.maxFrameBytes) {
          throw new BridgeProtocolError("Pi-family bridge frame size is invalid");
        }
        this.#acceptLine(line);
      }
    } catch (error) {
      this.#stop(error instanceof Error ? error : new BridgeProtocolError(String(error)), false);
    }
  }

  #acceptLine(line) {
    let frame;
    try {
      frame = JSON.parse(this.decoder.decode(line));
    } catch {
      throw new BridgeProtocolError("Pi-family bridge frame is not valid UTF-8 JSON");
    }
    validateObject(frame, "frame");
    if (frame.version !== BRIDGE_PROTOCOL_VERSION) {
      throw new BridgeProtocolError("Pi-family bridge version is unsupported");
    }
    if (typeof frame.type !== "string") {
      throw new BridgeProtocolError("Pi-family bridge frame type is invalid");
    }
    if (!this.sawHello) {
      if (frame.type !== "hello") {
        throw new BridgeProtocolError("Pi-family bridge first frame is not hello");
      }
      exactKeys(frame, ["version", "type", "role"]);
      if (frame.role !== this.peerRole) {
        throw new BridgeProtocolError("Pi-family bridge peer role is invalid");
      }
      this.sawHello = true;
      this.readyState = true;
      this.peerReady.resolve();
      return;
    }
    if (frame.type === "hello") throw new BridgeProtocolError("Pi-family bridge received duplicate hello");
    switch (frame.type) {
      case "request":
        this.#acceptRequest(frame, line.length);
        break;
      case "response":
        this.#acceptResponse(frame, line.length);
        break;
      case "cancel":
        this.#acceptCancel(frame);
        break;
      default:
        throw new BridgeProtocolError(`Pi-family bridge frame type ${JSON.stringify(frame.type)} is unknown`);
    }
  }

  #acceptRequest(frame, bytes) {
    exactKeys(frame, ["version", "type", "id", "method", "params"]);
    const sequence = parseRequestID(frame.id, this.peerRole);
    if (sequence !== this.nextInboundID + 1) {
      throw new BridgeProtocolError(`Pi-family bridge request id ${JSON.stringify(frame.id)} is duplicate or out of order`);
    }
    this.nextInboundID = sequence;
    if (!validMethod(frame.method)) throw new BridgeProtocolError("Pi-family bridge method is invalid");
    if (
      this.active.size >= this.limits.maxActiveCalls ||
      this.retainedBytes + this.pendingWriteBytes > this.limits.maxRetainedBytes - bytes
    ) {
      this.#sendDetachedFrame({
        version: BRIDGE_PROTOCOL_VERSION,
        type: "response",
        id: frame.id,
        error: { code: "busy", message: "Pi-family bridge handler capacity is exhausted" },
      });
      return;
    }

    const controller = new AbortController();
    const active = { controller, bytes, cancelled: false };
    this.active.set(frame.id, active);
    this.retainedBytes += bytes;
    let task;
    task = this.#runHandler(frame, active)
      .catch((error) => this.#stop(error, false))
      .finally(() => this.tasks.delete(task));
    this.tasks.add(task);
  }

  async #runHandler(frame, active) {
    try {
      let result;
      try {
        if (typeof this.handler !== "function") {
          throw new BridgeCallError("method_not_found", "Pi-family bridge method is not available");
        }
        result = await this.handler({
          method: frame.method,
          params: frame.params,
          signal: active.controller.signal,
          bridge: this,
        });
        if (result === undefined) result = null;
      } catch (error) {
        await this.#sendFrame({
          version: BRIDGE_PROTOCOL_VERSION,
          type: "response",
          id: frame.id,
          error: callError(error),
        });
        return;
      }
      await this.#sendFrame({
        version: BRIDGE_PROTOCOL_VERSION,
        type: "response",
        id: frame.id,
        result,
      });
    } finally {
      if (this.active.get(frame.id) === active) {
        this.active.delete(frame.id);
        this.retainedBytes -= active.bytes;
      }
    }
  }

  #acceptResponse(frame, bytes) {
    const hasResult = Object.hasOwn(frame, "result");
    const hasError = Object.hasOwn(frame, "error");
    if (hasResult === hasError) {
      throw new BridgeProtocolError("Pi-family bridge response must contain one of result or error");
    }
    exactKeys(frame, ["version", "type", "id", hasResult ? "result" : "error"]);
    parseRequestID(frame.id, this.role);
    const pending = this.pending.get(frame.id);
    if (!pending) {
      throw new BridgeProtocolError(`Pi-family bridge response id ${JSON.stringify(frame.id)} is unknown or duplicate`);
    }
    let error;
    if (hasError) {
      validateObject(frame.error, "response error");
      exactKeys(frame.error, ["code", "message"]);
      if (
        !validErrorCode(frame.error.code) ||
        typeof frame.error.message !== "string" ||
        Buffer.byteLength(frame.error.message) === 0 ||
        Buffer.byteLength(frame.error.message) > 1024
      ) {
        throw new BridgeProtocolError("Pi-family bridge response error is invalid");
      }
      error = new BridgeCallError(frame.error.code, frame.error.message);
    }
    if (pending.abandoned) {
      this.pending.delete(frame.id);
      return;
    }
    if (this.retainedBytes + this.pendingWriteBytes > this.limits.maxRetainedBytes - bytes) {
      throw new BridgeBusyError("Pi-family bridge retained response capacity is exhausted");
    }
    this.pending.delete(frame.id);
    pending.settled = true;
    this.retainedBytes += bytes;
    pending.resolve({ bridgeResponse: true, bytes, error, result: frame.result });
  }

  #acceptCancel(frame) {
    exactKeys(frame, ["version", "type", "id"]);
    const sequence = parseRequestID(frame.id, this.peerRole);
    if (sequence > this.nextInboundID) {
      throw new BridgeProtocolError(`Pi-family bridge cancellation id ${JSON.stringify(frame.id)} is in the future`);
    }
    const active = this.active.get(frame.id);
    if (active && !active.cancelled) {
      active.cancelled = true;
      active.controller.abort(new BridgeCallError("cancelled", "Pi-family bridge call was cancelled"));
    }
  }

  #stop(error, intentional) {
    if (this.stopped) {
      if (error && !this.intentional && !this.failure) this.failure = error;
      return;
    }
    this.stopped = true;
    this.intentional = intentional;
    if (error && !intentional) this.failure = error;
    const reason = error ?? new BridgeClosedError();
    this.peerReady.reject(reason);
    for (const pending of this.pending.values()) {
      if (!pending.abandoned && !pending.settled) {
        pending.settled = true;
        pending.reject(reason);
      }
    }
    this.pending.clear();
    for (const active of this.active.values()) active.controller.abort(reason);
    this.socket.destroy();
  }

  async #onClose() {
    if (this.closed) return;
    this.closed = true;
    if (!this.stopped) {
      this.#stop(new BridgeClosedError("Pi-family bridge peer closed the connection"), false);
    }
    const reason = this.#connectionError();
    for (const write of [...this.writes]) write.settle(reason);
    await Promise.allSettled([...this.tasks]);
    this.active.clear();
    this.buffer = Buffer.alloc(0);
    this.socket.off("data", this.onData);
    this.socket.off("error", this.onError);
    this.socket.off("end", this.onEnd);
    this.socket.off("close", this.onClose);
    this.doneState.resolve();
  }

  #withAbort(promise, signal, retire) {
    if (!signal) return promise;
    if (signal.aborted) return Promise.reject(abortReason(signal));
    return new Promise((resolve, reject) => {
      const aborted = () => {
        const reason = abortReason(signal);
        if (retire) this.#stop(reason, false);
        reject(reason);
      };
      signal.addEventListener("abort", aborted, { once: true });
      promise.then(
        (value) => {
          signal.removeEventListener("abort", aborted);
          resolve(value);
        },
        (error) => {
          signal.removeEventListener("abort", aborted);
          reject(error);
        },
      );
    });
  }
}

export async function connectBridge(socketPath, options = {}) {
  if (typeof socketPath !== "string" || socketPath.length === 0 || Buffer.byteLength(socketPath) > 4096) {
    throw new TypeError("Pi-family bridge socket path is invalid");
  }
  const { signal } = options;
  if (signal?.aborted) throw abortReason(signal);
  const socket = net.createConnection({ path: socketPath });
  try {
    await new Promise((resolve, reject) => {
      let settled = false;
      const settleAfterClose = (error) => {
        if (settled) return;
        settled = true;
        cleanup();
        if (socket.closed) {
          reject(error);
          return;
        }
        socket.once("close", () => reject(error));
        socket.destroy();
      };
      const cleanup = () => {
        signal?.removeEventListener("abort", aborted);
        socket.off("connect", connected);
        socket.off("error", failed);
      };
      const connected = () => {
        if (settled) return;
        settled = true;
        cleanup();
        resolve();
      };
      const failed = (error) => settleAfterClose(error);
      const aborted = () => settleAfterClose(abortReason(signal));
      socket.once("connect", connected);
      socket.once("error", failed);
      signal?.addEventListener("abort", aborted, { once: true });
      if (signal?.aborted) aborted();
    });
  } catch (error) {
    if (!socket.destroyed) socket.destroy();
    throw error;
  }
  let bridge;
  try {
    bridge = new PrivateBridge(socket, options);
  } catch (error) {
    const closed = socket.closed ? Promise.resolve() : onceSocketClose(socket);
    socket.destroy();
    await closed;
    throw error;
  }
  try {
    await bridge.ready(signal);
    return bridge;
  } catch (error) {
    await bridge.close().catch(() => {});
    throw error;
  }
}

function onceSocketClose(socket) {
  return new Promise((resolve) => socket.once("close", resolve));
}
