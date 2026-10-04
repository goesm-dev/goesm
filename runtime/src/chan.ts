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

import { ProgramExit, exitProcess, exiting, writeStd } from "./host.ts";
import { Goexit, plainPanic, runtimePanic, toPanic } from "./panic.ts";
import { fromJSString } from "./string.ts";

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
  capacity: number;
  zero: () => T;
  constructor(capacity: number, zero: () => T) {
    this.capacity = capacity;
    this.zero = zero;
  }
}

export function makeChan<T = any>(capacity: number, zero: () => T, elemSize = 1): Chan<T> {
  // 2^48 bytes is gc's maxAlloc on 64-bit platforms (elemSize is the
  // element's size there); the buffer here grows as needed.
  if (!(capacity >= 0 && capacity * elemSize < 2 ** 48)) runtimePanic("makechan: size out of range");
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

export function send<T = any>(ch: Chan<T> | null, v: T): void | Promise<void> {
  if (ch === null) return forever();
  if (trySend(ch, v)) return;
  return new Promise<void>((resolve, reject) => {
    ch.sendq.push({ sel: null, value: v, caseIndex: 0, wake: () => resolve(), fail: reject });
  });
}

export function recv<T = any>(ch: Chan<T> | null): [T, boolean] | Promise<[T, boolean]> {
  if (ch === null) return forever();
  const r = tryRecv(ch);
  if (r !== null) return r;
  return new Promise<[T, boolean]>((resolve, reject) => {
    ch.recvq.push({ sel: null, caseIndex: 0, wake: (v, ok) => resolve([v, ok]), fail: reject });
  });
}

// sendNow and recvNow are channel operations in functions goesm lowers as
// synchronous (internal/natives: syncFuncs), where they always complete at
// once. Blocking there would be a goesm bug, reported as a panic.
export function sendNow<T = any>(ch: Chan<T> | null, v: T): void {
  if (ch === null || !trySend(ch, v)) plainPanic("goesm: channel send would block in a synchronous function");
}

export function recvNow<T = any>(ch: Chan<T> | null): [T, boolean] {
  const r = ch === null ? null : tryRecv(ch);
  if (r === null) plainPanic("goesm: channel receive would block in a synchronous function");
  return r;
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

// With a default case select never blocks.
export function select(cases: SelectCase[], hasDefault: true): SelectResult;
export function select(cases: SelectCase[], hasDefault: boolean): SelectResult | Promise<SelectResult>;
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

// G is a goroutine's identity: its goroutine-local storage (see
// runtime.GetTraceContextFromGLS). curG is the running goroutine; generated
// code that needs it (see lower.Program.TracksGoroutines) restores it after
// every await, when the goroutine resumes.
export class G {
  traceContext: any = null;
  baggage: any = null;
}

export const mainG = new G();
let curG = mainG;

export function getG(): G {
  return curG;
}

// resumeG restores the goroutine g after an await and passes the awaited
// value through.
export function resumeG<T>(g: G, v: T): T {
  curG = g;
  return v;
}

// glsPropagate copies goroutine-local values to a new goroutine (registered
// by goesm's package runtime).
let glsPropagate: ((v: any) => any) | null = null;

export function setGLSPropagate(f: (v: any) => any): void {
  glsPropagate = f;
}

export function go(fn: (...args: any[]) => any, args: any[] = []): void {
  goroutines++;
  const g = new G();
  if (glsPropagate !== null) {
    if (curG.traceContext !== null) g.traceContext = glsPropagate(curG.traceContext);
    if (curG.baggage !== null) g.baggage = glsPropagate(curG.baggage);
  }
  queueMicrotask(() => {
    curG = g;
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

// crash ends the program on a panic nothing recovered, like Go: the panic
// goes to standard error and the process exits with status 2. Where there
// is no process to exit (browsers), the panic is reported as an uncaught
// error instead.
function crash(e: unknown): void {
  if (e instanceof ProgramExit) return;
  const p = toPanic(e);
  const g = globalThis as any;
  if (typeof g.process?.exit === "function") {
    const stack = (p.stack ?? "").split("\n").slice(1).join("\n");
    writeStd(2, fromJSString(p.message + "\n\ngoroutine 1 [running]:\n" + stack + "\n"));
    exitProcess(2);
  }
  if (typeof g.reportError === "function") g.reportError(p);
  else setTimeout(() => { throw p; });
}

// runMain runs the main function of the program's main package. As in Go,
// the program exits when main returns, even if other goroutines are still
// running. If the host's event loop runs dry while main is still blocked,
// nothing can wake it any more: that is Go's deadlock, reported the same way.
// Hosts without a process (browsers) keep running whatever is left.
// mainGoexit records that main called runtime.Goexit.
let mainGoexit = false;
let watching = false;

// deadlock runs when the event loop has run dry while the program is still
// initializing its packages or running main. beforeExit fires only then:
// not for an exit the program or a JavaScript callback asked for, nor for
// an uncaught JavaScript exception.
function deadlock(): void {
  if (exiting) return;
  writeStd(2, mainGoexit && goroutines === 0
    ? "fatal error: no goroutines (main called runtime.Goexit) - deadlock!\n"
    : "fatal error: all goroutines are asleep - deadlock!\n\ngoroutine 1 [running]:\nmain.main()\n");
  exitProcess(2);
}

// watchDeadlock reports a deadlock if the event loop runs dry before main
// returns. program.ts calls it before any package is initialized, so that a
// blocked init function is reported too.
export function watchDeadlock(): void {
  const proc = (globalThis as any).process;
  if (watching || typeof proc?.once !== "function") return;
  watching = true;
  proc.once("beforeExit", deadlock);
}

export function runMain(main: () => void | Promise<void>): void {
  const proc = (globalThis as any).process;
  watchDeadlock();
  const done = () => {
    proc?.off?.("beforeExit", deadlock);
    if (typeof proc?.exit === "function") exitProcess(0);
  };
  // runtime.Goexit in main ends the main goroutine; the others go on.
  const fail = (e: unknown) => {
    if (e instanceof Goexit) {
      mainGoexit = true;
      goroutines--;
      return;
    }
    proc?.off?.("beforeExit", deadlock);
    crash(e);
  };
  let r: void | Promise<void>;
  try {
    r = main();
  } catch (e) {
    fail(e);
    return;
  }
  if (r instanceof Promise) r.then(done, fail);
  else done();
}

// crashOnUncaught makes an exception nothing caught end the program like an
// unrecovered panic (program.ts).
export function crashOnUncaught(): void {
  const proc = (globalThis as any).process;
  if (typeof proc?.on === "function") proc.on("uncaughtException", crash);
}
