import { useState, useCallback } from "react";
import { createStore, type Row } from "./store";

const store = createStore();

export default function App() {
  const [data, setData] = useState<Row[]>([]);
  const [selected, setSelected] = useState<number | null>(null);

  const run = useCallback(() => {
    setData(store.buildData(1000));
    setSelected(null);
  }, []);

  const runLots = useCallback(() => {
    setData(store.buildData(10000));
    setSelected(null);
  }, []);

  const add = useCallback(() => {
    setData((d) => d.concat(store.buildData(1000)));
  }, []);

  const update = useCallback(() => {
    setData((d) => {
      const next = d.slice();
      for (let i = 0; i < next.length; i += 10) {
        next[i] = { ...next[i], label: next[i].label + " !!!" };
      }
      return next;
    });
  }, []);

  const clear = useCallback(() => {
    setData([]);
    setSelected(null);
  }, []);

  const swapRows = useCallback(() => {
    setData((d) => {
      if (d.length < 999) return d;
      const next = d.slice();
      const tmp = next[1];
      next[1] = next[998];
      next[998] = tmp;
      return next;
    });
  }, []);

  const remove = useCallback((id: number) => {
    setData((d) => d.filter((row) => row.id !== id));
  }, []);

  const select = useCallback((id: number) => {
    setSelected(id);
  }, []);

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
          {data.map((row) => (
            <tr key={row.id} className={row.id === selected ? "danger" : ""}>
              <td className="col-id">{row.id}</td>
              <td className="col-label">
                <a onClick={() => select(row.id)}>{row.label}</a>
              </td>
              <td className="col-remove">
                <a onClick={() => remove(row.id)}>x</a>
              </td>
              <td className="col-spacer"></td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
