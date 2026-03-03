<script>
  import { onMount } from 'svelte'
  import { LoadConfig, SaveConfig } from '../../wailsjs/go/main/App'

  export let onClose = () => {}

  let cfg = {
    name: '',
    tone: 'genki',
    encourage_freq: 'mid',
    monologue: true,
    always_on_top: true,
    mute: false,
    model: 'gemma3:4b',
  }

  onMount(async () => {
    cfg = await LoadConfig()
  })

  async function save() {
    await SaveConfig(cfg)
    onClose()
  }
</script>

<div class="settings">
  <h3>設定</h3>

  <label>
    呼び名
    <input type="text" bind:value={cfg.name} />
  </label>

  <label>
    口調
    <select bind:value={cfg.tone}>
      <option value="genki">元気</option>
      <option value="calm">落ち着き</option>
      <option value="polite">丁寧</option>
      <option value="tsundere">ツンデレ</option>
    </select>
  </label>

  <label>
    励まし頻度
    <select bind:value={cfg.encourage_freq}>
      <option value="low">少ない</option>
      <option value="mid">普通</option>
      <option value="high">多い</option>
    </select>
  </label>

  <label>
    <input type="checkbox" bind:checked={cfg.monologue} />
    独り言
  </label>

  <label>
    <input type="checkbox" bind:checked={cfg.always_on_top} />
    最前面
  </label>

  <label>
    <input type="checkbox" bind:checked={cfg.mute} />
    ミュート
  </label>

  <label>
    モデル
    <input type="text" bind:value={cfg.model} />
  </label>

  <div class="buttons">
    <button on:click={save}>保存</button>
    <button on:click={onClose}>閉じる</button>
  </div>
</div>

<style>
  .settings {
    background: white;
    border: 1px solid #ccc;
    border-radius: 8px;
    padding: 12px;
    font-size: 12px;
    width: 180px;
  }

  h3 {
    margin: 0 0 8px;
    font-size: 14px;
  }

  label {
    display: flex;
    flex-direction: column;
    margin-bottom: 6px;
    gap: 2px;
  }

  input[type="text"], select {
    padding: 2px 4px;
    font-size: 12px;
  }

  label:has(input[type="checkbox"]) {
    flex-direction: row;
    align-items: center;
    gap: 4px;
  }

  .buttons {
    display: flex;
    gap: 4px;
    margin-top: 8px;
  }

  button {
    flex: 1;
    padding: 4px;
    font-size: 12px;
    cursor: pointer;
  }
</style>
