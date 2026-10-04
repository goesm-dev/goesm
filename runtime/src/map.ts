// Go maps. Keys are hashed with Go equality (see hashKey), so struct,
// array and interface keys, NaN keys and nil-map behaviour follow Go rather
// than JS object/Map semantics. A nil map is JS null.
//
// Iteration order: Go randomises it; the PoC iterates in insertion order.
// Programs may not depend on either, but tests comparing against native Go
// must not depend on order.

import { hashKey } from "./iface.ts";
import { plainPanic, runtimePanic } from "./panic.ts";
import { Kind, Type } from "./types.ts";

// A map whose keys are booleans, integers, strings or channels, which JS
// Map compares as Go does (SameValueZero, which only differs from Go
// equality for floats), is direct: entries maps each key to its value.
// Other maps hash keys with hashKey and map the hash to a [key, value]
// entry, which keeps the key itself (a float key's sign, the dynamic
// value of an interface key).
export class GoMap<K, V> {
  entries = new Map<any, any>();
  declare keyType: Type;
  declare direct: boolean;
  constructor(keyType: Type) {
    this.keyType = keyType;
    this.direct = directKey(keyType);
  }
}

function directKey(t: Type): boolean {
  switch (t.kind) {
    case Kind.Bool: case Kind.Int: case Kind.Int8: case Kind.Int16: case Kind.Int32: case Kind.Int64:
    case Kind.Uint: case Kind.Uint8: case Kind.Uint16: case Kind.Uint32: case Kind.Uint64: case Kind.Uintptr:
    case Kind.String: case Kind.Chan:
      return true;
  }
  return false;
}

export type M<K, V> = GoMap<K, V> | null;

// makeMap makes a map; the size hint of make(map[K]V, n) is ignored.
export function makeMap<K = any, V = any>(keyType: Type, _hint?: number): GoMap<K, V> {
  return new GoMap<K, V>(keyType);
}

export function mapLit<K = any, V = any>(keyType: Type, kvs: Array<[K, V]>): GoMap<K, V> {
  const m = new GoMap<K, V>(keyType);
  for (const [k, v] of kvs) mapSet(m, k, v);
  return m;
}

export function mapGet<K = any, V = any>(m: M<K, V>, k: K, zero: () => V): V {
  if (m === null) return zero();
  if (m.direct) {
    const v = m.entries.get(k);
    return v !== undefined || m.entries.has(k) ? v : zero();
  }
  const e = m.entries.get(hashKey(m.keyType, k));
  return e === undefined ? zero() : e[1];
}

export function mapLookup<K = any, V = any>(m: M<K, V>, k: K, zero: () => V): [V, boolean] {
  if (m === null) return [zero(), false];
  if (m.direct) {
    const v = m.entries.get(k);
    return v !== undefined || m.entries.has(k) ? [v, true] : [zero(), false];
  }
  const e = m.entries.get(hashKey(m.keyType, k));
  return e === undefined ? [zero(), false] : [e[1], true];
}

export function mapSet<K = any, V = any>(m: M<K, V>, k: K, v: V): void {
  if (m === null) plainPanic("assignment to entry in nil map");
  if (m.direct) {
    m.entries.set(k, v);
    return;
  }
  const h = hashKey(m.keyType, k);
  const e = m.entries.get(h);
  if (e !== undefined) {
    // gc stores the new key too (NeedKeyUpdate): m[+0] = v after m[-0]
    // leaves the key +0. For other keys the two are indistinguishable.
    e[0] = k;
    e[1] = v;
  } else m.entries.set(h, [k, v]);
}

export function mapDelete<K = any, V = any>(m: M<K, V>, k: K): void {
  if (m === null) return;
  m.entries.delete(m.direct ? k : hashKey(m.keyType, k));
}

export function mapLen(m: M<any, any>): number {
  return m === null ? 0 : m.entries.size;
}

export function mapClear(m: M<any, any>): void {
  if (m !== null) m.entries.clear();
}

// mapRange iterates over live [key, value] entries; entries deleted during
// iteration are not produced, entries added may or may not be (both allowed
// by the Go spec).
export function mapRange<K = any, V = any>(m: M<K, V>): IterableIterator<[K, V]> {
  if (m === null) return noEntries.values();
  return m.direct ? m.entries.entries() : m.entries.values();
}

const noEntries: [any, any][] = [];
