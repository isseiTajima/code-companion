<script>
  import { onMount, createEventDispatcher } from 'svelte'
  import { defaultConfig, loadConfig, saveConfig, pullModel, onPullProgress, listOllamaModels, deleteModel, createSakuraModel } from '../lib/wails'

  export let onClose = () => {}
  const dispatch = createEventDispatcher()

  const CATALOG_JA = [
    { id: 'schroneko/gemma-2-2b-jpn-it', label: 'Gemma 2 JPN 2B', desc: '~2.8GB · 日本語特化' },
    { id: 'gemma3:4b',                   label: 'Gemma 3 4B',     desc: '~3.3GB · 多言語' },
    { id: 'dsasai/llama3-elyza-jp-8b',   label: 'ELYZA JP 8B',    desc: '~4.9GB · 日本語最高品質' },
  ]
  const CATALOG_GLOBAL = [
    { id: 'gemma3:4b', label: 'Gemma 3 4B', desc: '~3.3GB' },
  ]

  function sakuraModelName(id) {
    const name = id.includes('/') ? id.split('/').pop() : id
    return 'sakura-' + name
  }

  let cfg = { ...defaultConfig }
  let installedModels = []
  let pullingModel = ''
  let pullPercent = 0
  let pullStatus = ''
  let otherModelsOpen = false

  onMount(async () => {
    cfg = await loadConfig()
    installedModels = await listOllamaModels()
    onPullProgress((p) => {
      if (p.error) {
        pullStatus = `${t.errorPrefix} ${p.error}`
        pullingModel = ''
      } else if (p.total && p.completed) {
        pullPercent = Math.round((p.completed / p.total) * 100)
        pullStatus = `${pullPercent}%`
      } else if (p.status !== 'success') {
        pullStatus = p.status ?? ''
      }
    })
  })

  async function startPull(modelId) {
    pullingModel = modelId
    pullPercent = 0
    pullStatus = '開始...'
    await pullModel(modelId)
    pullStatus = t.creating
    const sakuraName = await createSakuraModel(modelId)
    installedModels = [...installedModels, modelId]
    if (sakuraName) {
      installedModels = [...installedModels, sakuraName]
      cfg.model = sakuraName
    } else {
      cfg.model = modelId
    }
    pullingModel = ''
    pullStatus = ''
  }

  function selectModel(modelId) {
    const sn = sakuraModelName(modelId)
    cfg.model = installedModels.includes(sn) ? sn : modelId
    otherModelsOpen = false
  }

  async function removeModel(modelId) {
    const sn = sakuraModelName(modelId)
    await deleteModel(modelId)
    if (installedModels.includes(sn)) await deleteModel(sn)
    installedModels = installedModels.filter(n => n !== modelId && n !== sn && !n.startsWith(modelId + ':'))
    if (cfg.model === modelId || cfg.model === sn) cfg.model = ''
  }

  const I18N = {
    ja: {
      title: '設定', yourName: 'あなたの呼び名', namePlaceholder: '開発者',
      tone: 'サクラの口調', toneGenki: '元気', toneCalm: '落ち着き', tonePolite: '丁寧', toneTsundere: 'ツンデレ',
      language: '言語',
      freq: 'お話の頻度', freqLow: '控えめ', freqMid: 'ふつう', freqHigh: 'お喋り',
      llmSection: '脳 (LLM) 設定', backend: 'バックエンド',
      backendOllama: 'Ollama (ローカル)', backendClaude: 'Claude (API)', backendRouter: '自動フォールバック',
      localModel: 'ローカルモデル', otherModels: '他のモデル', activeLabel: '使用中', notSet: '未設定',
      displaySection: '表示設定', position: '表示位置', posTopRight: '右上', posBottomRight: '右下',
      monologue: '独り言', alwaysOnTop: '最前面', mute: 'ミュート', autoStart: '自動起動',
      autoStartTitle: 'ログイン時に自動でサクラが起動します',
      size: '大きさ', opacity: '不透明度', save: '保存して適用',
      creating: 'キャラ設定中...', errorPrefix: 'エラー:',
    },
    en: {
      title: 'Settings', yourName: 'Your name', namePlaceholder: 'Developer',
      tone: 'Sakura\'s tone', toneGenki: 'Energetic', toneCalm: 'Calm', tonePolite: 'Polite', toneTsundere: 'Tsundere',
      language: 'Language',
      freq: 'Speech frequency', freqLow: 'Less', freqMid: 'Normal', freqHigh: 'More',
      llmSection: 'LLM Settings', backend: 'Backend',
      backendOllama: 'Ollama (Local)', backendClaude: 'Claude (API)', backendRouter: 'Auto fallback',
      localModel: 'Local model', otherModels: 'Other models', activeLabel: 'Active', notSet: 'Not set',
      displaySection: 'Display', position: 'Position', posTopRight: 'Top right', posBottomRight: 'Bottom right',
      monologue: 'Monologue', alwaysOnTop: 'Always on top', mute: 'Mute', autoStart: 'Auto start',
      autoStartTitle: 'Launch Sakura automatically at login',
      size: 'Size', opacity: 'Opacity', save: 'Save & Apply',
      creating: 'Configuring...', errorPrefix: 'Error:',
    },
  }

  $: t = cfg.language === 'en' ? I18N.en : I18N.ja
  $: catalog = cfg.language === 'en' ? CATALOG_GLOBAL : CATALOG_JA
  $: currentEntry = catalog.find(m => cfg.model === m.id || cfg.model === sakuraModelName(m.id)) ?? null
  $: otherCatalog = catalog.filter(m => cfg.model !== m.id && cfg.model !== sakuraModelName(m.id))

  async function save() {
    await saveConfig(cfg)
    dispatch('saved', cfg)
    onClose()
  }
</script>

<div class="settings" onclick={(e) => e.stopPropagation()} onmousedown={(e) => e.stopPropagation()}>
  <div class="header">
    <h3>{t.title}</h3>
    <button class="close-x" onclick={onClose}>×</button>
  </div>

  <div class="scroll-area">
    <label>
      {t.yourName}
      <input type="text" bind:value={cfg.user_name} placeholder={t.namePlaceholder} />
    </label>

    <label>
      {t.tone}
      <select bind:value={cfg.tone}>
        <option value="genki">{t.toneGenki}</option>
        <option value="calm">{t.toneCalm}</option>
        <option value="polite">{t.tonePolite}</option>
        <option value="tsundere">{t.toneTsundere}</option>
      </select>
    </label>

    <label>
      {t.language}
      <select bind:value={cfg.language}>
        <option value="ja">日本語</option>
        <option value="en">English</option>
      </select>
    </label>

    <label>
      {t.freq}
      <select bind:value={cfg.speech_frequency}>
        <option value={1}>{t.freqLow}</option>
        <option value={2}>{t.freqMid}</option>
        <option value={3}>{t.freqHigh}</option>
      </select>
    </label>

    <div class="section-title">{t.llmSection}</div>

    <label>
      {t.backend}
      <select bind:value={cfg.llm_backend}>
        <option value="ollama">{t.backendOllama}</option>
        <option value="claude">{t.backendClaude}</option>
        <option value="router">{t.backendRouter}</option>
      </select>
    </label>

    {#if cfg.llm_backend === 'ollama' || cfg.llm_backend === 'router'}
      <label>Ollama Endpoint
        <input type="text" bind:value={cfg.ollama_endpoint} />
      </label>
      <div class="model-section-label">{t.localModel}</div>
      <div class="model-list">
        {#if currentEntry}
          <div class="model-item selected">
            <div class="model-info">
              <span class="model-name">{currentEntry.label}</span>
              <span class="model-desc">{currentEntry.desc}</span>
            </div>
            {#if pullingModel === currentEntry.id}
              <span class="model-badge pulling">{pullStatus}</span>
            {:else}
              <span class="model-badge active">{t.activeLabel}</span>
            {/if}
          </div>
        {:else}
          <div class="model-item selected">
            <div class="model-info">
              <span class="model-name">{cfg.model || t.notSet}</span>
            </div>
            <span class="model-badge active">{t.activeLabel}</span>
          </div>
        {/if}
        <button class="other-models-toggle" onclick={() => otherModelsOpen = !otherModelsOpen}>
          {t.otherModels} {otherModelsOpen ? '▲' : '▼'}
        </button>
        {#if otherModelsOpen}
          {#each otherCatalog as m}
            {@const installed = installedModels.some(n => n === m.id || n === sakuraModelName(m.id) || n.startsWith(m.id + ':'))}
            {@const isPulling = pullingModel === m.id}
            <div class="model-item" onclick={() => installed && selectModel(m.id)}>
              <div class="model-info">
                <span class="model-name">{m.label}</span>
                <span class="model-desc">{m.desc}</span>
              </div>
              {#if isPulling}
                <span class="model-badge pulling">{pullStatus}</span>
              {:else if installed}
                <div class="model-actions">
                  <button class="model-select-btn" onclick={(e) => { e.stopPropagation(); selectModel(m.id); }}>✓</button>
                  <button class="model-del-btn" onclick={(e) => { e.stopPropagation(); removeModel(m.id); }}>×</button>
                </div>
              {:else}
                <button class="model-dl-btn" disabled={!!pullingModel} onclick={(e) => { e.stopPropagation(); startPull(m.id); }}>DL</button>
              {/if}
            </div>
          {/each}
        {/if}
      </div>
      {#if pullingModel && pullPercent > 0}
        <div class="pull-bar"><div class="pull-fill" style="width:{pullPercent}%"></div></div>
      {/if}
      {#if pullStatus && pullStatus.startsWith(t.errorPrefix)}
        <div class="pull-error">{pullStatus}</div>
      {/if}
    {/if}

    {#if cfg.llm_backend === 'claude' || cfg.llm_backend === 'router'}
      <label>
        Claude API Key
        <input type="password" bind:value={cfg.anthropic_api_key} placeholder="sk-ant-..." />
      </label>
    {/if}

    {#if cfg.llm_backend === 'router'}
      <label>
        Gemini API Key
        <input type="password" bind:value={cfg.gemini_api_key} placeholder="AIza..." />
      </label>
    {/if}

    <div class="section-title">{t.displaySection}</div>

    <label>
      {t.position}
      <select bind:value={cfg.window_position}>
        <option value="top-right">{t.posTopRight}</option>
        <option value="bottom-right">{t.posBottomRight}</option>
      </select>
    </label>

    <div class="checkbox-group">
      <label>
        <input type="checkbox" bind:checked={cfg.monologue} />
        {t.monologue}
      </label>
      <label>
        <input type="checkbox" bind:checked={cfg.always_on_top} />
        {t.alwaysOnTop}
      </label>
      <label>
        <input type="checkbox" bind:checked={cfg.mute} />
        {t.mute}
      </label>
      <label title={t.autoStartTitle}>
        <input type="checkbox" bind:checked={cfg.auto_start} />
        {t.autoStart}
      </label>
    </div>

    <div class="row">
      <label style="flex: 1;">
        {t.size} ({Math.round(cfg.scale * 100)}%)
        <input type="range" min="0.8" max="2.0" step="0.1" bind:value={cfg.scale} />
      </label>
      <label style="flex: 1;">
        {t.opacity} ({Math.round(cfg.independent_window_opacity * 100)}%)
        <input type="range" min="0.05" max="1" step="0.05" bind:value={cfg.independent_window_opacity} />
      </label>
    </div>
  </div>

  <div class="buttons">
    <button class="save-btn" onclick={save}>{t.save}</button>
  </div>
</div>

<style>
  .settings {
    background: rgba(255, 255, 255, 0.98);
    backdrop-filter: blur(10px);
    border: 1px solid rgba(0, 0, 0, 0.2);
    border-radius: 10px;
    padding: 8px 10px;
    font-size: 10px;
    width: 210px;
    max-height: 260px; /* 300pxのウィンドウ内に収める */
    display: flex;
    flex-direction: column;
    box-shadow: 0 8px 24px rgba(0, 0, 0, 0.2);
    color: #333;
  }

  .header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 6px;
    border-bottom: 1px solid #eee;
    padding-bottom: 2px;
  }

  .close-x {
    background: none;
    border: none;
    font-size: 16px;
    cursor: pointer;
    color: #999;
    padding: 0 2px;
  }

  .scroll-area {
    overflow-y: auto;
    flex: 1;
    padding-right: 4px;
  }

  h3 {
    margin: 0;
    font-size: 11px;
    font-weight: bold;
  }

  .section-title {
    font-weight: bold;
    margin: 8px 0 4px;
    color: #e91e63;
    font-size: 9px;
    text-transform: uppercase;
  }

  label {
    display: flex;
    flex-direction: column;
    margin-bottom: 6px;
    gap: 1px;
  }

  .row {
    display: flex;
    gap: 6px;
  }

  .checkbox-group {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 2px;
    margin-bottom: 6px;
  }

  input[type='text'],
  input[type='password'],
  select {
    padding: 3px;
    font-size: 10px;
    border: 1px solid #ddd;
    border-radius: 4px;
    background: white;
  }

  label:has(input[type='checkbox']) {
    flex-direction: row;
    align-items: center;
    gap: 4px;
    margin-bottom: 2px;
    cursor: pointer;
  }

  .buttons {
    margin-top: 8px;
  }

  .save-btn {
    width: 100%;
    padding: 5px;
    background: #e91e63;
    color: white;
    border: none;
    border-radius: 6px;
    font-weight: bold;
    cursor: pointer;
    font-size: 10px;
  }

  .save-btn:hover {
    background: #d81b60;
  }

  .model-section-label { font-size: 9px; color: #666; margin-bottom: 3px; }
  .other-models-toggle { background: none; border: none; width: 100%; text-align: left; font-size: 9px; color: #aaa; padding: 2px 0; cursor: pointer; }
  .model-list { display: flex; flex-direction: column; gap: 2px; margin-bottom: 6px; }
  .model-item { display: flex; align-items: center; justify-content: space-between; padding: 4px 6px; border: 1px solid #eee; border-radius: 5px; cursor: default; }
  .model-item.selected { border-color: #e91e63; background: #fff0f5; }
  .model-info { display: flex; flex-direction: column; gap: 1px; }
  .model-name { font-size: 10px; font-weight: bold; color: #111; }
  .model-desc { font-size: 9px; color: #666; }
  .model-badge { font-size: 9px; white-space: nowrap; }
  .model-badge.active { color: #e91e63; font-weight: bold; }
  .model-badge.pulling { color: #888; }
  .model-actions { display: flex; gap: 3px; }
  .model-select-btn { background: none; border: 1px solid #e91e63; color: #e91e63; border-radius: 4px; font-size: 9px; padding: 1px 5px; cursor: pointer; }
  .model-del-btn { background: none; border: 1px solid #ccc; color: #999; border-radius: 4px; font-size: 9px; padding: 1px 5px; cursor: pointer; }
  .model-del-btn:hover { border-color: #d32f2f; color: #d32f2f; }
  .model-dl-btn { background: #e91e63; color: white; border: none; border-radius: 4px; font-size: 9px; font-weight: bold; padding: 2px 6px; cursor: pointer; }
  .model-dl-btn:disabled { background: #ccc; cursor: default; }
  .pull-bar { height: 3px; background: #f0f0f0; border-radius: 2px; overflow: hidden; margin-bottom: 4px; }
  .pull-fill { height: 100%; background: #e91e63; transition: width 0.3s; }
  .pull-error { font-size: 9px; color: #d32f2f; margin-bottom: 4px; }
</style>
