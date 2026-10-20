// The JavaScript side of package jscompat (js.go).

export const cos = Math.cos, sin = Math.sin, tan = Math.tan, exp = Math.exp, log = Math.log, pow = Math.pow, atan2 = Math.atan2;

export function normalize(s: string, form: string): string {
  return s.normalize(form);
}

// A RegExp of package jscompat: one for single matches and a global one
// for all matches, so that neither keeps a lastIndex between calls.
interface Pair {
  one: RegExp;
  all: RegExp;
}

export function compile(pattern: string, flags: string): Pair {
  const one = flags.replace(/[gy]/g, "");
  return { one: new RegExp(pattern, one), all: new RegExp(pattern, one + "g") };
}

export function test(re: Pair, s: string): boolean {
  return re.one.test(s);
}

export function exec(re: Pair, s: string): string[] | null {
  const m = re.one.exec(s);
  return m && Array.from(m, (x) => x ?? "");
}

export function matchAll(re: Pair, s: string, n: number): string[] {
  const out: string[] = [];
  for (const m of s.matchAll(re.all)) {
    if (n >= 0 && out.length >= n) break;
    out.push(m[0]);
  }
  return out;
}

export function replace(re: Pair, s: string, repl: string): string {
  return s.replace(re.all, repl);
}

export function source(re: Pair): string {
  return re.one.source;
}
