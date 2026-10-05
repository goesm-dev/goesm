// remark and rehype's version of ../../markdown/markdown.go.
import rehypeStringify from "rehype-stringify";
import remarkParse from "remark-parse";
import remarkRehype from "remark-rehype";
import { unified } from "unified";

const processor = unified().use(remarkParse).use(remarkRehype).use(rehypeStringify);

export function Render(src) {
  return String(processor.processSync(src));
}
