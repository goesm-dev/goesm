// @goesm/runtime: the semantic support library for TypeScript lowered from Go.
//
// Generated code imports this module as `$rt`. The runtime only implements Go
// semantics that JS does not have natively; it never decides Go typing
// questions, which goesm settles at compile time with go/types.

export * from "./types";
export * from "./iface";
export * from "./panic";
export * from "./slice";
export * from "./map";
export * from "./ptr";
export * from "./string";
export * from "./int";
export * from "./chan";
export * from "./interop";

import { toJSString } from "./string";

// print/println builtins. Go writes these to stderr; the PoC uses the console.
export function println(...args: any[]): void {
  console.log(args.map((a) => (typeof a === "string" ? toJSString(a) : String(a))).join(" "));
}
