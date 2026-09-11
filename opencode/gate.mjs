// SPDX-License-Identifier: MIT

// Bounded removable subscribers, unlike racing a permanently pending shared
// promise once per cancelled native call. The establishing operation is owned
// and joined separately by its native/plugin lifetime.
export class ReadyGate {
  #done = false;
  #error;
  #value;
  #waiters = new Set();

  settle(error, value) {
    if (this.#done) return;
    this.#done = true;
    this.#error = error;
    this.#value = value;
    for (const finish of [...this.#waiters]) finish(error, value);
  }

  wait(signal) {
    if (signal?.aborted) return Promise.reject(signal.reason || new Error("native call cancelled"));
    if (this.#done) return this.#error ? Promise.reject(this.#error) : Promise.resolve(this.#value);
    if (this.#waiters.size >= 256) return Promise.reject(new Error("Sessionbus pending readiness limit reached"));
    return new Promise((resolve, reject) => {
      const finish = (error, value) => {
        this.#waiters.delete(finish);
        signal?.removeEventListener("abort", abort);
        if (error) reject(error); else resolve(value);
      };
      const abort = () => finish(signal.reason || new Error("native call cancelled"));
      this.#waiters.add(finish);
      signal?.addEventListener("abort", abort, { once: true });
    });
  }
}
