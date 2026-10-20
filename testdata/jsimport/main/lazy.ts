// Imported dynamically by the lazy //goesm:import directives of promises.go.
(globalThis as any).lazyLoaded = true;

export const name = "lazy";

export function twice(n: number): number {
  return 2 * n;
}

export async function later(s: string): Promise<string> {
  await new Promise((r) => setTimeout(r, 1));
  return s + "!";
}
