// Goroutines, channels and select.
//
// Execution model (PoC): a goroutine is an async JS function started on the
// microtask queue. Functions that may block (channel ops, select, calls to
// blocking functions, determined by goesm's whole-program blocking analysis)
// are lowered to `async` functions and every blocking point is an `await`.
// Non-blocking code stays synchronous. This is cooperative scheduling: a
// goroutine only yields at blocking points (no preemption).
//
// Channel operations return synchronously when they can complete immediately
// and return a Promise otherwise, so the fast path never allocates a Promise.
// Blocking is represented by wait queues on the channel, which is what select,
// close and (later) deadlock detection are built on; it is not delegated to
// the JS event loop's own semantics.

import { Goexit, plainPanic, runtimePanic, toPanic } from "./panic";

interface Waiter {
  sel: { done: boolean } | null;
  value?: any;
  caseIndex: number;
  wake: (v: any, ok: boolean) => void;
  fail: (e: unknown) => void;
}

export class Chan<T> {
  buf: T[] = [];
  closed = false;
  recvq: Waiter[] = [];
  sendq: Waiter[] = [];
  constructor(public capacity: number, public zero: () => T) {}
}

export function makeChan<T>(capacity: number, zero: () => T): Chan<T> {
  if (capacity < 0) runtimePanic("makechan: size out of range");
  return new Chan(capacity, zero);
}

function dequeue(q: Waiter[]): Waiter | undefined {
  while (q.length > 0) {
    const w = q.shift()!;
    if (w.sel !== null) {
      if (w.sel.done) continue;
      w.sel.done = true;
    }
    return w;
  }
  return undefined;
}

function trySend<T>(ch: Chan<T>, v: T): boolean {
  if (ch.closed) plainPanic("send on closed channel");
  const r = dequeue(ch.recvq);
  if (r !== undefined) {
    r.wake(v, true);
    return true;
  }
  if (ch.buf.length < ch.capacity) {
    ch.buf.push(v);
    return true;
  }
  return false;
}

function tryRecv<T>(ch: Chan<T>): [T, boolean] | null {
  if (ch.buf.length > 0) {
    const v = ch.buf.shift()!;
    const s = dequeue(ch.sendq);
    if (s !== undefined) {
      ch.buf.push(s.value);
      s.wake(undefined, true);
    }
    return [v, true];
  }
  const s = dequeue(ch.sendq);
  if (s !== undefined) {
    s.wake(undefined, true);
    return [s.value, true];
  }
  if (ch.closed) return [ch.zero(), false];
  return null;
}

const forever = () => new Promise<never>(() => {});

export function send<T>(ch: Chan<T> | null, v: T): void | Promise<void> {
  if (ch === null) return forever();
  if (trySend(ch, v)) return;
  return new Promise<void>((resolve, reject) => {
    ch.sendq.push({ sel: null, value: v, caseIndex: 0, wake: () => resolve(), fail: reject });
  });
}

export function recv<T>(ch: Chan<T> | null): [T, boolean] | Promise<[T, boolean]> {
  if (ch === null) return forever();
  const r = tryRecv(ch);
  if (r !== null) return r;
  return new Promise<[T, boolean]>((resolve, reject) => {
    ch.recvq.push({ sel: null, caseIndex: 0, wake: (v, ok) => resolve([v, ok]), fail: reject });
  });
}

export function close(ch: Chan<any> | null): void {
  if (ch === null) plainPanic("close of nil channel");
  if (ch.closed) plainPanic("close of closed channel");
  ch.closed = true;
  for (let r = dequeue(ch.recvq); r !== undefined; r = dequeue(ch.recvq)) r.wake(ch.zero(), false);
  for (let s = dequeue(ch.sendq); s !== undefined; s = dequeue(ch.sendq)) {
    try {
      plainPanic("send on closed channel");
    } catch (e) {
      s.fail(e);
    }
  }
}

export function chanLen(ch: Chan<any> | null): number {
  return ch === null ? 0 : ch.buf.length;
}

export function chanCap(ch: Chan<any> | null): number {
  return ch === null ? 0 : ch.capacity;
}

// A select case: [channel, isSend, valueToSend].
export type SelectCase = [Chan<any> | null, boolean, any];
// Result: [chosen case index or -1 for default, received value, ok].
export type SelectResult = [number, any, boolean];

export function select(cases: SelectCase[], hasDefault: boolean): SelectResult | Promise<SelectResult> {
  // Go picks uniformly among ready cases.
  const order = cases.map((_, i) => i);
  for (let i = order.length - 1; i > 0; i--) {
    const j = Math.floor(Math.random() * (i + 1));
    [order[i], order[j]] = [order[j], order[i]];
  }
  for (const i of order) {
    const [ch, isSend, v] = cases[i];
    if (ch === null) continue;
    if (isSend) {
      if (trySend(ch, v)) return [i, undefined, false];
    } else {
      const r = tryRecv(ch);
      if (r !== null) return [i, r[0], r[1]];
    }
  }
  if (hasDefault) return [-1, undefined, false];
  return new Promise<SelectResult>((resolve, reject) => {
    const sel = { done: false };
    for (const i of order) {
      const [ch, isSend, v] = cases[i];
      if (ch === null) continue;
      const w: Waiter = {
        sel,
        value: v,
        caseIndex: i,
        wake: (rv, ok) => resolve(isSend ? [i, undefined, false] : [i, rv, ok]),
        fail: reject,
      };
      (isSend ? ch.sendq : ch.recvq).push(w);
    }
  });
}

// go starts fn(...args) as a new goroutine. An unrecovered panic in a
// goroutine terminates a Go program; here it is reported as an uncaught
// error on the host (process exit in Node, console error in browsers).
// runtime.Goexit ends the goroutine quietly.
let goroutines = 1; // the main goroutine (the host's own execution)

export function numGoroutine(): number {
  return goroutines;
}

export function go(fn: (...args: any[]) => any, args: any[] = []): void {
  goroutines++;
  queueMicrotask(() => {
    let r: any;
    try {
      r = fn(...args);
    } catch (e) {
      exit(e);
      return;
    }
    if (r instanceof Promise) r.then(() => exit(undefined), exit);
    else exit(undefined);
  });
}

function exit(e: unknown): void {
  goroutines--;
  if (e !== undefined && !(e instanceof Goexit)) crash(e);
}

function crash(e: unknown): void {
  const p = toPanic(e);
  const g = globalThis as any;
  if (typeof g.reportError === "function") g.reportError(p);
  else setTimeout(() => { throw p; });
}
