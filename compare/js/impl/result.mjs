// neverthrow's version of ../../result/result.go.
import { err, ok, Result } from "neverthrow";

const missing = (name) => err(`${name}: missing field`);

function field(fields, name) {
  return fields.has(name) ? ok(fields.get(name)) : missing(name);
}

// Go's strconv.Atoi: an optional sign and decimal digits.
const intRE = /^[+-]?\d+$/;

function number(fields, name, lo, hi) {
  return field(fields, name)
    .andThen((s) => (intRE.test(s) ? ok(Number(s)) : err(`${name}: strconv.Atoi: parsing ${JSON.stringify(s)}: invalid syntax`)))
    .andThen((n) => (n < lo || n > hi ? err(`${name}: ${n}: out of range`) : ok(n)));
}

function fields(line) {
  const m = new Map();
  for (const part of line.split(";")) {
    const i = part.indexOf("=");
    if (i < 0) return err(`malformed field ${JSON.stringify(part)}`);
    m.set(part.slice(0, i).trim(), part.slice(i + 1).trim());
  }
  return ok(m);
}

export function ParseOrder(line) {
  return fields(line).andThen((f) =>
    field(f, "sku")
      .andThen((sku) => (sku.length < 3 || !sku.includes("-") ? err(`sku ${JSON.stringify(sku)}: out of range`) : ok(sku)))
      .andThen((sku) => Result.combine([number(f, "qty", 1, 99), number(f, "price", 0, 1_000_000)]).map(([qty, price]) => ({ SKU: sku, Qty: qty, Price: price }))),
  );
}

export function Check(lines) {
  const r = { valid: 0, total: 0, errors: [] };
  lines.forEach((line, i) => {
    ParseOrder(line).match(
      (o) => {
        r.valid++;
        r.total += o.Qty * o.Price;
      },
      (e) => r.errors.push(`line ${i + 1}: ${e}`),
    );
  });
  return r;
}
