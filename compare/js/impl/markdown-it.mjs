// markdown-it's version of ../../markdown/markdown.go.
import MarkdownIt from "markdown-it";

const md = new MarkdownIt("commonmark");

export function Render(src) {
  return md.render(src);
}
