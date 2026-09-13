<script lang="ts">
  import { createStore, type Row } from "./store";

  const store = createStore();

  let data = $state<Row[]>([]);
  let selected = $state<number | null>(null);

  function run() {
    data = store.buildData(1000);
    selected = null;
  }
  function runLots() {
    data = store.buildData(10000);
    selected = null;
  }
  function add() {
    data = data.concat(store.buildData(1000));
  }
  function update() {
    const next = data.slice();
    for (let i = 0; i < next.length; i += 10) {
      next[i] = { ...next[i], label: next[i].label + " !!!" };
    }
    data = next;
  }
  function clear() {
    data = [];
    selected = null;
  }
  function swapRows() {
    if (data.length < 999) return;
    const next = data.slice();
    const tmp = next[1];
    next[1] = next[998];
    next[998] = tmp;
    data = next;
  }
  function remove(id: number) {
    data = data.filter((row) => row.id !== id);
  }
  function select(id: number) {
    selected = id;
  }
</script>

<div id="main">
  <div id="controls">
    <button id="run" onclick={run}>Create 1,000 rows</button>
    <button id="runlots" onclick={runLots}>Create 10,000 rows</button>
    <button id="add" onclick={add}>Append 1,000 rows</button>
    <button id="update" onclick={update}>Update every 10th row</button>
    <button id="clear" onclick={clear}>Clear</button>
    <button id="swaprows" onclick={swapRows}>Swap Rows</button>
  </div>
  <table>
    <tbody id="tbody">
      {#each data as row (row.id)}
        <tr class={row.id === selected ? "danger" : ""}>
          <td class="col-id">{row.id}</td>
          <td class="col-label">
            <a onclick={() => select(row.id)}>{row.label}</a>
          </td>
          <td class="col-remove">
            <a onclick={() => remove(row.id)}>x</a>
          </td>
          <td class="col-spacer"></td>
        </tr>
      {/each}
    </tbody>
  </table>
</div>
