// keep holds the function Go gives it, for the test to pass back to Go.
export function keep(f: unknown): void {
  (globalThis as { goesmKept?: unknown }).goesmKept = f;
}
