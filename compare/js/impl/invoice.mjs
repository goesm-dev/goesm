// smol-toml's version of ../../invoice/invoice.go.
import { parse, stringify } from "smol-toml";

function summary(f) {
  let subtotal = 0;
  let receipts = 0;
  let foreign = 0;
  for (const l of f.lines) {
    subtotal += l.qty * l.price;
    receipts += l.receipts.length;
    for (const r of l.receipts) if (r.currency !== "JPY") foreign += r.amount;
  }
  const tax = f.invoice.taxed ? Math.trunc((subtotal * f.invoice.tax_percent) / 100) : 0;
  return [f.invoice.id, f.buyer.name, f.products[f.lines[0].product], f.lines.length, receipts, subtotal, tax, foreign].join("|");
}

export function Open(src) {
  return summary(parse(src));
}

export function Resave(src) {
  return summary(parse(stringify(parse(src))));
}
