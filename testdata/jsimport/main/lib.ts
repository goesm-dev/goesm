// Functions the Go program imports with //goesm:import.

export function greet(name: string): string {
  return `こんにちは、${name}さん`;
}

export function sum(xs: number[]): number {
  return xs.reduce((a, b) => a + b, 0);
}

export interface Item {
  name: string;
  price: number;
  tags: string[] | null;
}

export function describe(item: Item): string {
  return `${item.name}: ${item.price} [${(item.tags ?? []).join(",")}]`;
}

export function makeItems(n: number): Item[] {
  return Array.from({ length: n }, (_, i) => ({ name: `item${i}`, price: (i + 1) * 100, tags: i % 2 ? ["odd"] : null }));
}

export function parsePrice(s: string): number {
  const n = Number(s);
  if (Number.isNaN(n)) throw new Error(`not a price: ${s}`);
  return n;
}

export function mustBePositive(n: number): void {
  if (n <= 0) throw new RangeError(`${n} is not positive`);
}

export async function later(ms: number, v: string): Promise<string> {
  await new Promise((r) => setTimeout(r, ms));
  return v + "!";
}

export async function rejectLater(): Promise<string> {
  await new Promise((r) => setTimeout(r, 1));
  throw new Error("rejected");
}

export function mapStrings(xs: string[], f: (s: string, i: number) => string): string[] {
  return xs.map((x, i) => f(x, i));
}

export function join(sep: string, ...parts: unknown[]): string {
  return parts.map(String).join(sep);
}

export function bytes(n: number): Uint8Array {
  return Uint8Array.from({ length: n }, (_, i) => i * 3);
}

export function byteSum(b: Uint8Array): number {
  return b.reduce((a, x) => a + x, 0);
}

export function big(x: bigint): bigint {
  return x * 2n;
}

export function divmod(a: number, b: number): [number, number] {
  return [Math.floor(a / b), a % b];
}

export function counts(words: string[]): Record<string, number> {
  const m: Record<string, number> = {};
  for (const w of words) m[w] = (m[w] ?? 0) + 1;
  return m;
}

export function decode(s: string): unknown {
  return JSON.parse(s);
}

export class Counter {
  n = 0;
  add(d: number): number {
    return (this.n += d);
  }
}

export const VERSION = "1.2.3";

export default function shout(s: string): string {
  return s.toUpperCase();
}

export function stringify(v: unknown): string {
  return JSON.stringify(v);
}

export function keys(m: Record<string, number>): string {
  return Object.keys(m).sort().join(",") + " " + (Object.getPrototypeOf(m) === Object.prototype);
}

export function throwValue(v: unknown): void {
  throw v;
}
