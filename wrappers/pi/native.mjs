// SPDX-License-Identifier: MIT

export const customMessageType = "sessionbus.message";

function nativeIdentity(ctx) {
  const id = ctx.sessionManager.getSessionId();
  if (typeof id !== "string" || !id.length || Buffer.byteLength(id) > 256 || /[\s\0]/u.test(id)) {
    throw new Error("Pi omitted its native session identity");
  }
  return id;
}

export function describeNative(pi, ctx) {
  const session_id = nativeIdentity(ctx);
  const name = pi.getSessionName() ?? "";
  if (typeof name !== "string" || typeof ctx.cwd !== "string" || !ctx.cwd.length) {
    throw new Error("Pi native session metadata is invalid");
  }
  return { session_id, name, cwd: ctx.cwd };
}

// Pi d981de1's idle branch appends synchronously, despite the void extension
// API. The native leaf is the acknowledgement; extension message handlers do
// not receive this path. Never await between the idle check and leaf proof.
export function appendNative(pi, ctx, delivery) {
  if (!delivery || delivery.session_id !== nativeIdentity(ctx)) {
    throw new Error("Pi delivery belongs to a different native session");
  }
  if (typeof delivery.message_id !== "string" || !delivery.message_id.length ||
      Buffer.byteLength(delivery.message_id) > 256 ||
      typeof delivery.body !== "string" || Buffer.byteLength(delivery.body) > (1 << 20)) {
    throw new Error("invalid Pi native delivery");
  }
  if (!ctx.isIdle()) return { accepted: false, reason: "busy" };
  const parent = ctx.sessionManager.getLeafId();
  pi.sendMessage({
    customType: customMessageType,
    content: delivery.body,
    display: true,
    details: { message_id: delivery.message_id },
  }, { triggerTurn: false });
  const leaf = ctx.sessionManager.getLeafEntry();
  if (nativeIdentity(ctx) !== delivery.session_id || !leaf || leaf.type !== "custom_message" ||
      leaf.customType !== customMessageType || leaf.details?.message_id !== delivery.message_id ||
      leaf.content !== delivery.body || leaf.parentId !== parent || typeof leaf.id !== "string" ||
      !leaf.id.length || leaf.id === parent) {
    throw new Error("Pi did not confirm the native message append");
  }
  return { accepted: true, entry_id: leaf.id };
}
