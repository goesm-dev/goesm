// @vue/reactivity's version of ../../reactive/reactive.go's Bench.
import { computed, effect, ref } from "@vue/reactivity";

export function Bench(width, depth, rounds) {
  const refs = new Array(width);
  const ends = new Array(width);
  for (let i = 0; i < width; i++) {
    const r = ref(i);
    refs[i] = r;
    let prev = r;
    for (let j = 0; j < depth; j++) {
      const p = prev;
      const k = j;
      prev = computed(() => p.value + k);
    }
    ends[i] = prev;
  }
  const left = computed(() => refs[0].value * 2);
  const right = computed(() => refs[0].value + 1);
  const parity = computed(() => (left.value + right.value) % 2);
  const total = computed(() => {
    let s = 0;
    for (const c of ends) s += c.value;
    return s;
  });
  let runs = 0;
  let last = 0;
  const runner = effect(() => {
    runs++;
    last = total.value * 10 + parity.value;
  });
  for (let r = 0; r < rounds; r++) refs[(r * 7) % width].value = r * 3;
  runner.effect.stop();
  return [runs, last];
}
