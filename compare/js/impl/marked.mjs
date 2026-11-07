// marked's version of ../../gfm/gfm.go.
import { Marked } from "marked";

const md = new Marked({ gfm: true });

export function Render(src) {
  return md.parse(src, { async: false });
}
