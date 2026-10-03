// Go maps. Keys are hashed with Go equality (see hashKey), so struct,
// array and interface keys, NaN keys and nil-map behaviour follow Go rather
// than JS object/Map semantics. A nil map is JS null.
//
// Iteration order: Go randomises it; the PoC iterates in insertion order.
// Programs may not depend on either, but tests comparing against native Go
// must not depend on order.

import { hashKey } from "./iface";
import { plainPanic, runtimePanic } from "./panic";
import { Type } from "./types";

export class GoMap<K, V> {
  entries = new Map<any, [K, V]>();
  constructor(public keyType: Type) {}
}

export type M<K, V> = GoMap<K, V> | null;

export function makeMap<K, V>(keyType: Type): GoMap<K, V> {
  return new GoMap<K, V>(keyType);
}

export function mapLit<K, V>(keyType: Type, kvs: Array<[K, V]>): GoMap<K, V> {
  const m = new GoMap<K, V>(keyType);
  for (const [k, v] of kvs) m.entries.set(hashKey(keyType, k), [k, v]);
  return m;
}

export function mapGet<K, V>(m: M<K, V>, k: K, zero: () => V): V {
  if (m === null) return zero();
  const e = m.entries.get(hashKey(m.keyType, k));
  return e === undefined ? zero() : e[1];
}

export function mapLookup<K, V>(m: M<K, V>, k: K, zero: () => V): [V, boolean] {
  if (m === null) return [zero(), false];
  const e = m.entries.get(hashKey(m.keyType, k));
  return e === undefined ? [zero(), false] : [e[1], true];
}

export function mapSet<K, V>(m: M<K, V>, k: K, v: V): void {
  if (m === null) plainPanic("assignment to entry in nil map");
  const h = hashKey(m.keyType, k);
  const e = m.entries.get(h);
  if (e !== undefined) e[1] = v; // Go keeps the original key on overwrite
  else m.entries.set(h, [k, v]);
}

export function mapDelete<K, V>(m: M<K, V>, k: K): void {
  if (m === null) return;
  m.entries.delete(hashKey(m.keyType, k));
}

export function mapLen(m: M<any, any>): number {
  return m === null ? 0 : m.entries.size;
}

export function mapClear(m: M<any, any>): void {
  if (m !== null) m.entries.clear();
}

// mapRange yields live entries; entries deleted during iteration are not
// produced, entries added may or may not be (both allowed by the Go spec).
export function* mapRange<K, V>(m: M<K, V>): Generator<[K, V]> {
  if (m === null) return;
  for (const e of m.entries.values()) yield e;
}
