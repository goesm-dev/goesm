// @goesm/runtime: the semantic support library for TypeScript lowered from Go.
//
// Generated code imports this module as `$rt`. The runtime only implements Go
// semantics that JS does not have natively; it never decides Go typing
// questions, which goesm settles at compile time with go/types.

export * from "./types.ts";
export * from "./iface.ts";
export * from "./panic.ts";
export * from "./slice.ts";
export * from "./map.ts";
export * from "./ptr.ts";
export * from "./string.ts";
export * from "./int.ts";
export * from "./chan.ts";
export * from "./interop.ts";

import { toJSString } from "./string.ts";

// print/println builtins. Go writes these to stderr; the PoC uses the console.
export function println(...args: any[]): void {
  console.log(args.map((a) => (typeof a === "string" ? toJSString(a) : String(a))).join(" "));
}
