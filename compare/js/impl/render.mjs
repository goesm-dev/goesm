// React's version of ../../render/render.go.
import { createElement as h } from "react";
import { renderToString } from "react-dom/server.browser";

const yen = (n) => "¥" + n.toLocaleString("en-US");

function Product({ item }) {
  return h(
    "li",
    { className: item.soldOut ? "item sold-out" : "item", "data-id": item.id },
    h("h2", null, item.name),
    h("span", { className: "price" }, yen(item.price)),
    item.tags.length > 0 ? h("ul", { className: "tags" }, item.tags.map((t, i) => h("li", { key: i }, t))) : null,
    item.soldOut ? h("em", null, "Sold out") : null,
  );
}

function Page({ title, items }) {
  return h(
    "main",
    { className: "page" },
    h("h1", null, title),
    h("p", { className: "count" }, `${items.length} items`),
    h("ul", { className: "items" }, items.map((item) => h(Product, { key: item.id, item }))),
  );
}

export function Render(title, items) {
  return renderToString(h(Page, { title, items }));
}
