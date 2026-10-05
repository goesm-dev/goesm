// luxon's version of ../../datetime/datetime.go.
import { DateTime } from "luxon";

const opts = { setZone: true };

function parse(iso) {
  const t = DateTime.fromISO(iso, opts);
  if (!t.isValid) throw new Error(t.invalidExplanation);
  return t;
}

export function Shift(iso, days) {
  return parse(iso).plus({ days }).toISO({ suppressMilliseconds: true });
}

export function DaysBetween(a, b) {
  return Math.trunc(parse(b).diff(parse(a), "days").days);
}

export function Label(iso) {
  return parse(iso).toFormat("ccc, LLL d yyyy HH:mm", { locale: "en-US" });
}

export function MonthGrid(year, month) {
  const first = DateTime.utc(year, month, 1);
  const start = first.minus({ days: first.weekday - 1 });
  const days = new Array(42);
  for (let i = 0; i < 42; i++) days[i] = start.plus({ days: i }).toISODate();
  return days;
}
