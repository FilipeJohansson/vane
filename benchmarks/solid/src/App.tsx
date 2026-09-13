import { createSignal, For } from "solid-js";
import { createStore as createWordStore, type Row } from "./store";

const wordStore = createWordStore();

function App() {
  const [data, setData] = createSignal<Row[]>([]);
  const [selected, setSelected] = createSignal<number | null>(null);

  const run = () => {
    setData(wordStore.buildData(1000));
    setSelected(null);
  };
  const runLots = () => {
    setData(wordStore.buildData(10000));
    setSelected(null);
  };
  const add = () => {
    setData(data().concat(wordStore.buildData(1000)));
  };
  const update = () => {
    const next = data().slice();
    for (let i = 0; i < next.length; i += 10) {
      next[i] = { ...next[i], label: next[i].label + " !!!" };
    }
    setData(next);
  };
  const clear = () => {
    setData([]);
    setSelected(null);
  };
  const swapRows = () => {
    const d = data();
    if (d.length < 999) return;
    const next = d.slice();
    const tmp = next[1];
    next[1] = next[998];
    next[998] = tmp;
    setData(next);
  };
  const remove = (id: number) => {
    setData(data().filter((row) => row.id !== id));
  };
  const select = (id: number) => setSelected(id);

  return (
    <div id="main">
      <div id="controls">
        <button id="run" onClick={run}>Create 1,000 rows</button>
        <button id="runlots" onClick={runLots}>Create 10,000 rows</button>
        <button id="add" onClick={add}>Append 1,000 rows</button>
        <button id="update" onClick={update}>Update every 10th row</button>
        <button id="clear" onClick={clear}>Clear</button>
        <button id="swaprows" onClick={swapRows}>Swap Rows</button>
      </div>
      <table>
        <tbody id="tbody">
          <For each={data()}>
            {(row) => (
              <tr classList={{ danger: row.id === selected() }}>
                <td class="col-id">{row.id}</td>
                <td class="col-label">
                  <a onClick={() => select(row.id)}>{row.label}</a>
                </td>
                <td class="col-remove">
                  <a onClick={() => remove(row.id)}>x</a>
                </td>
                <td class="col-spacer"></td>
              </tr>
            )}
          </For>
        </tbody>
      </table>
    </div>
  );
}

export default App;
