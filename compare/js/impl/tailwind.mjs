// Tailwind CSS's version of ../../utility/utility.go: compile a stylesheet
// that imports Tailwind's theme and utilities, then build it for a page's
// class names.
import { compile } from "tailwindcss";

const css = `@import "tailwindcss/theme.css" layer(theme);\n@import "tailwindcss/utilities.css" layer(utilities);`;

export async function Build(theme, candidates) {
  const compiler = await compile(css, {
    loadStylesheet: async (id, base) => ({ path: id, base, content: id === "tailwindcss/theme.css" ? theme : "@tailwind utilities;" }),
  });
  return compiler.build(candidates);
}
