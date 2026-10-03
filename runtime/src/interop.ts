// JS boundary helpers. toJS converts a Go value, guided by its type
// descriptor, into plain JS data shaped like encoding/json's output. It is used
// by the golden tests (native Go JSON == goesm ESM toJS) and is a first step
// towards a typed JS ABI for exported functions.

import { Kind, Type } from "./types";
import { toJSString } from "./string";
import { GoMap } from "./map";
import { Slice } from "./slice";

export function toJS(t: Type, v: any): any {
  switch (t.kind) {
    case Kind.String:
      return toJSString(v);
    case Kind.Slice: {
      if (v === null) return null;
      const s = v as Slice<any>;
      const items = s.$array.slice(s.$offset, s.$offset + s.$length);
      if (t.elem!.kind === Kind.Uint8) return base64(items);
      return items.map((x) => toJS(t.elem!, x));
    }
    case Kind.Array:
      return (v as any[]).map((x) => toJS(t.elem!, x));
    case Kind.Map: {
      if (v === null) return null;
      const o: Record<string, any> = {};
      for (const [k, x] of (v as GoMap<any, any>).entries.values()) {
        o[t.key!.kind === Kind.String ? toJSString(k) : String(k)] = toJS(t.elem!, x);
      }
      return o;
    }
    case Kind.Struct: {
      const o: Record<string, any> = {};
      for (const f of t.fields) {
        if (f.pkgPath !== "") continue; // unexported
        let name = f.name;
        const m = /json:"([^"]*)"/.exec(f.tag);
        if (m) {
          const tagName = m[1].split(",")[0];
          if (tagName === "-") continue;
          if (tagName) name = tagName;
        }
        if (f.embedded && f.type.kind === Kind.Struct && !m) {
          Object.assign(o, toJS(f.type, v[f.prop]));
          continue;
        }
        o[name] = toJS(f.type, v[f.prop]);
      }
      return o;
    }
    case Kind.Pointer:
      if (v === null) return null;
      return t.elem!.kind === Kind.Struct || t.elem!.kind === Kind.Array ? toJS(t.elem!, v) : toJS(t.elem!, v.v);
    case Kind.Interface:
      return v === null ? null : toJS(v.t, v.v);
    case Kind.Chan: case Kind.Func: case Kind.UnsafePointer:
      return v === null ? null : `<${t.str}>`;
  }
  return v;
}

function base64(bytes: number[]): string {
  let bin = "";
  for (const b of bytes) bin += String.fromCharCode(b);
  return btoa(bin);
}
