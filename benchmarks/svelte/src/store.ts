// Row data generation, matching js-framework-benchmark's own word lists and
// random-pick formula (frameworks/keyed/vanillajs/src/Main.js in
// krausest/js-framework-benchmark), duplicated per framework app the same
// way that reference repo itself does - each framework folder there carries
// its own copy, not a shared module.

const adjectives = [
  "pretty", "large", "big", "small", "tall", "short", "long", "handsome",
  "plain", "quaint", "clean", "elegant", "easy", "angry", "crazy", "helpful",
  "mushy", "odd", "unsightly", "adorable", "important", "inexpensive",
  "cheap", "expensive", "fancy",
];
const colours = [
  "red", "yellow", "blue", "green", "pink", "brown", "purple", "brown",
  "white", "black", "orange",
];
const nouns = [
  "table", "chair", "house", "bbq", "desk", "car", "pony", "cookie",
  "sandwich", "burger", "pizza", "mouse", "keyboard",
];

function randomIndex(max: number): number {
  return Math.round(Math.random() * 1000) % max;
}

function makeLabel(): string {
  return (
    adjectives[randomIndex(adjectives.length)] +
    " " +
    colours[randomIndex(colours.length)] +
    " " +
    nouns[randomIndex(nouns.length)]
  );
}

export interface Row {
  id: number;
  label: string;
}

export function createStore() {
  let nextId = 1;
  return {
    buildData(count: number): Row[] {
      const rows = new Array<Row>(count);
      for (let i = 0; i < count; i++) {
        rows[i] = { id: nextId++, label: makeLabel() };
      }
      return rows;
    },
  };
}
