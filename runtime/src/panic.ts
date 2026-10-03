// panic / recover / defer.
//
// A Go panic is a JS exception of class GoPanic carrying the panic value as a
// Go interface value. Functions that contain `defer` are lowered to
//
//   const $d = new $rt.Defers();
//   $body: try { ... } catch (e) { $d.fail(e); } finally { $d.run(); }
//   return <results>;
//
// so deferred calls run in LIFO order on both normal return and panic, a
// deferred call can recover() and the function then returns its (possibly
// modified) named results, exactly like Go.

import { Iface } from "./iface";
import { Kind, Type, addMethods, funcOf, named, setUnderlying, types } from "./types";
import { toJSString } from "./string";

// runtime.Error values. Their dynamic type implements error and runtime.Error
// so user code can recover() them and call Error().
export const runtimeErrorType: Type = named("runtime", "Error");
setUnderlying(runtimeErrorType, types.string);
addMethods(runtimeErrorType, {
  Error: [(v: string) => "runtime error: " + v, funcOf([], [types.string], false)],
  RuntimeError: [() => undefined, funcOf([], [], false)],
});

// runtime.plainError: runtime errors whose message has no prefix in Go
// (nil map assignment, channel misuse).
export const plainErrorType: Type = named("runtime", "plainError");
setUnderlying(plainErrorType, types.string);
addMethods(plainErrorType, {
  Error: [(v: string) => v, funcOf([], [types.string], false)],
  RuntimeError: [() => undefined, funcOf([], [], false)],
});

export function plainPanic(msg: string): never {
  throw new GoPanic(new Iface(plainErrorType, msg));
}

// *runtime.TypeAssertionError: its message has no "runtime error: " prefix.
export const typeAssertionErrorType: Type = named("runtime", "TypeAssertionError");
setUnderlying(typeAssertionErrorType, types.string);
addMethods(typeAssertionErrorType, {
  Error: [(v: string) => v, funcOf([], [types.string], false)],
  RuntimeError: [() => undefined, funcOf([], [], false)],
});

export const panicNilErrorType: Type = named("runtime", "PanicNilError");
setUnderlying(panicNilErrorType, types.string);
addMethods(panicNilErrorType, {
  Error: [() => "panic called with nil argument (use runtime.PanicNilError)", funcOf([], [types.string], false)],
  RuntimeError: [() => undefined, funcOf([], [], false)],
});

export class GoPanic extends Error {
  constructor(public value: Iface) {
    super("panic: " + formatPanicValue(value));
    this.name = "GoPanic";
  }
}

export function formatPanicValue(v: Iface | null): string {
  if (v === null) return "nil";
  const err = v.t.methods.get("Error");
  if (err) {
    return toJSString(err.fn(v.v));
  }
  const str = v.t.methods.get("String");
  if (str) return toJSString(str.fn(v.v));
  switch (v.t.kind) {
    case Kind.String:
      return v.t.named ? `${v.t.str}(${JSON.stringify(toJSString(v.v))})` : toJSString(v.v);
    case Kind.Bool: case Kind.Int: case Kind.Int8: case Kind.Int16: case Kind.Int32: case Kind.Int64:
    case Kind.Uint: case Kind.Uint8: case Kind.Uint16: case Kind.Uint32: case Kind.Uint64: case Kind.Uintptr:
    case Kind.Float32: case Kind.Float64:
      return v.t.named ? `${v.t.str}(${v.v})` : String(v.v);
  }
  return `(${v.t.str}) ${String(v.v)}`;
}

export function panic(v: Iface | null): never {
  if (v === null) v = new Iface(panicNilErrorType, "");
  throw new GoPanic(v);
}

export function runtimePanic(msg: string): never {
  throw new GoPanic(new Iface(runtimeErrorType, msg));
}

// toPanic converts anything thrown into a GoPanic. JS TypeErrors arise from
// touching null (nil pointers, nil maps on read paths that skipped a check);
// they are reported as Go's nil dereference runtime error.
export function toPanic(e: unknown): GoPanic {
  if (e instanceof GoPanic) return e;
  let p: GoPanic;
  if (e instanceof TypeError) {
    p = new GoPanic(new Iface(runtimeErrorType, "invalid memory address or nil pointer dereference"));
  } else if (e instanceof RangeError && /call stack/i.test(e.message)) {
    p = new GoPanic(new Iface(runtimeErrorType, "stack overflow (JS call stack exhausted)"));
  } else {
    p = new GoPanic(new Iface(runtimeErrorType, "js: " + String(e)));
  }
  if (e instanceof Error && e.stack) p.stack = p.message + "\n" + e.stack.split("\n").slice(1).join("\n");
  (p as any).cause = e;
  return p;
}

// The defer frame whose deferred call is currently executing (synchronously).
// recover() consults it. See ARCHITECTURE.md for the known gap: Go only lets
// recover() work when called directly by the deferred function; the PoC
// accepts any call made synchronously during that deferred call.
let current: Defers | null = null;

// Goexit is thrown by runtime.Goexit: deferred calls run, recover() does
// not stop it, and the goroutine ends without crashing the program.
export class Goexit {}

export class Defers {
  private list: Array<() => any> = [];
  panicking: GoPanic | null = null;
  exiting: Goexit | null = null;

  defer(fn: () => any): void {
    this.list.push(fn);
  }

  fail(e: unknown): void {
    if (e instanceof Goexit) this.exiting = e;
    else this.panicking = toPanic(e);
  }

  run(): void {
    while (this.list.length > 0) {
      const fn = this.list.pop()!;
      const prev = current;
      current = this;
      try {
        fn();
      } catch (e) {
        this.fail(e);
      } finally {
        current = prev;
      }
    }
    if (this.panicking) throw this.panicking;
    if (this.exiting) throw this.exiting;
  }

  async runAsync(): Promise<void> {
    while (this.list.length > 0) {
      const fn = this.list.pop()!;
      const prev = current;
      current = this;
      let r: any;
      try {
        r = fn();
      } catch (e) {
        this.fail(e);
      } finally {
        current = prev;
      }
      if (r instanceof Promise) {
        try {
          await r;
        } catch (e) {
          this.fail(e);
        }
      }
    }
    if (this.panicking) throw this.panicking;
    if (this.exiting) throw this.exiting;
  }
}

export function recover(): Iface | null {
  const f = current;
  if (f === null || f.panicking === null) return null;
  const v = f.panicking.value;
  f.panicking = null;
  return v;
}
