<script>
  import { onMount, createEventDispatcher } from 'svelte'
  import { defaultConfig, loadConfig, saveConfig, pullModel, onPullProgress, listOllamaModels, deleteModel, createSakuraModel } from '../lib/wails'

  export let onClose = () => {}
  export let onOpenReview = () => {}
  const dispatch = createEventDispatcher()

  const CATALOG_JA = [
    { id: 'qwen3.5:9b',                  label: 'Qwen 3.5 9B',    desc: '~5.5GB · 推奨（最新・万能）' },
    { id: 'swallow:8b',                  label: 'Swallow 8B',     desc: '~5GB · 日本語特化（自然さ重視）' },
    { id: 'qwen2.5:3b',                  label: 'Qwen 2.5 3B',    desc: '~1.9GB · 最軽量' },
    { id: 'schroneko/gemma-2-2b-jpn-it', label: 'Gemma 2 JPN 2B', desc: '~2.8GB · 安定版' },
  ]
  const CATALOG_GLOBAL = [
    { id: 'qwen3.5:9b', label: 'Qwen 3.5 9B', desc: '~5.5GB · Recommended' },
    { id: 'swallow:8b', label: 'Swallow 8B',  desc: '~5GB · Japanese Specialization' },
    { id: 'qwen2.5:3b', label: 'Qwen 2.5 3B', desc: '~1.9GB · Smallest' },
  ]

  function sakuraModelName(id) {
    const name = id.includes('/') ? id.split('/').pop() : id
    return 'sakura-' + name
  }

  let cfg = { ...defaultConfig }
  let installedModels = []
  let ollamaConnected = true
  let pullingModel = ''
  let pullPercent = 0
  let pullStatus = ''
  let otherModelsOpen = false

  async function refreshModels() {
    const result = await listOllamaModels()
    ollamaConnected = result !== null
    installedModels = result ?? []
  }

  onMount(async () => {
    cfg = await loadConfig()
    await refreshModels()
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
      tone: 'サクラの口調', toneGenki: '元気', toneCalm: '落ち着き', toneOneesan: 'お姉さん', toneTsundere: 'ツンデレ',
      dialect: '方言', dialectStandard: '標準語', dialectHakata: '博多弁', dialectKyoto: '京都弁', dialectKansai: '関西弁',
      language: '言語',
      freq: 'お話の頻度', freqLow: '控えめ', freqMid: 'ふつう', freqHigh: 'お喋り', freqDev: '超高頻度(dev)',
      llmSection: '脳 (LLM) 設定', backend: 'バックエンド',
      backendOllama: 'Ollama (ローカル)', backendClaude: 'Claude (API)',
      backendGemini: 'Gemini (API)', backendRouter: '自動フォールバック',
      localModel: 'ローカルモデル', otherModels: '他のモデル', activeLabel: '使用中', notSet: '未設定',
      displaySection: '表示設定', position: '表示位置', posTopRight: '右上', posBottomRight: '右下',
      monologue: '独り言', alwaysOnTop: '最前面', mute: 'ミュート', autoStart: '自動起動',
      autoStartTitle: 'ログイン時に自動でサクラが起動します',
      size: '大きさ', opacity: '不透明度', save: '保存して適用',
      betaSection: 'ベータ機能',
      creating: 'キャラ設定中...', errorPrefix: 'エラー:',
      newsSection: 'ニュースフィード', newsAddPlaceholder: 'RSS URL を追加...',
      newsReset: 'デフォルトに戻す',
      newsCategories: { tech: 'テック', general: '一般', game: 'ゲーム', anime: 'アニメ' },
      ollamaWarning: '⚠️ Ollama が起動していません', ollamaRetry: '再接続',
    },
    en: {
      title: 'Settings', yourName: 'Your name', namePlaceholder: 'Developer',
      tone: 'Sakura\'s tone', toneGenki: 'Energetic', toneCalm: 'Calm', toneOneesan: 'Big Sis', toneTsundere: 'Tsundere',
      dialect: 'Dialect', dialectStandard: 'Standard', dialectHakata: 'Hakata', dialectKyoto: 'Kyoto', dialectKansai: 'Kansai',
      language: 'Language',
      freq: 'Speech frequency', freqLow: 'Less', freqMid: 'Normal', freqHigh: 'More', freqDev: 'Ultra (dev)',
      llmSection: 'LLM Settings', backend: 'Backend',
      backendOllama: 'Ollama (Local)', backendClaude: 'Claude (API)',
      backendGemini: 'Gemini (API)', backendRouter: 'Auto fallback',
      localModel: 'Local model', otherModels: 'Other models', activeLabel: 'Active', notSet: 'Not set',
      displaySection: 'Display', position: 'Position', posTopRight: 'Top right', posBottomRight: 'Bottom right',
      monologue: 'Monologue', alwaysOnTop: 'Always on top', mute: 'Mute', autoStart: 'Auto start',
      autoStartTitle: 'Launch Sakura automatically at login',
      size: 'Size', opacity: 'Opacity', save: 'Save & Apply',
      betaSection: 'BETA Functions',
      creating: 'Configuring...', errorPrefix: 'Error:',
      newsSection: 'News Feeds', newsAddPlaceholder: 'Add RSS URL...',
      newsReset: 'Reset to defaults',
      newsCategories: { tech: 'Tech', general: 'General', game: 'Gaming', anime: 'Anime' },
      ollamaWarning: '⚠️ Ollama is not running', ollamaRetry: 'Reconnect',
    },
  }

  // デフォルトフィード（リセット用・表示用）
  const DEFAULT_FEEDS = {
    ja: {
      tech:    ['https://hnrss.org/frontpage', 'https://gigazine.net/news/rss_2.0/', 'https://rss.itmedia.co.jp/rss/2.0/techplus.xml'],
      general: ['https://www.nhk.or.jp/rss/news/cat0.xml'],
      game:    ['https://automaton-media.com/feed/', 'https://www.4gamer.net/rss/index.xml'],
      anime:   ['https://natalie.mu/comic/feed/news'],
    },
    en: {
      tech:    ['https://hnrss.org/frontpage', 'https://feeds.feedburner.com/TechCrunch'],
      general: ['https://feeds.bbci.co.uk/news/world/rss.xml', 'https://feeds.reuters.com/reuters/topNews'],
      game:    ['https://www.ign.com/articles/feed/all'],
      anime:   [],
    },
  }

  const NEWS_CATEGORIES = ['tech', 'general', 'game', 'anime']

  let newsInputs = {}
  let newsOpen = false

  function currentFeeds(category) {
    const lang = cfg.language === 'en' ? 'en' : 'ja'
    const custom = cfg.news_feeds?.[category]
    return custom?.length ? custom : DEFAULT_FEEDS[lang]?.[category] ?? []
  }

  function isCustomized(category) {
    return !!(cfg.news_feeds?.[category]?.length)
  }

  function addFeed(category) {
    const url = newsInputs[category]?.trim()
    if (!url) return
    if (!cfg.news_feeds) cfg.news_feeds = {}
    const current = cfg.news_feeds[category] ?? [...(DEFAULT_FEEDS[cfg.language === 'en' ? 'en' : 'ja']?.[category] ?? [])]
    if (!current.includes(url)) {
      cfg.news_feeds = { ...cfg.news_feeds, [category]: [...current, url] }
    }
    newsInputs = { ...newsInputs, [category]: '' }
  }

  function removeFeed(category, url) {
    if (!cfg.news_feeds) cfg.news_feeds = {}
    const lang = cfg.language === 'en' ? 'en' : 'ja'
    const base = cfg.news_feeds[category]?.length ? cfg.news_feeds[category] : DEFAULT_FEEDS[lang]?.[category] ?? []
    cfg.news_feeds = { ...cfg.news_feeds, [category]: base.filter(u => u !== url) }
  }

  function resetFeeds(category) {
    if (!cfg.news_feeds) return
    const { [category]: _, ...rest } = cfg.news_feeds
    cfg.news_feeds = rest
  }

  $: t = cfg.language === 'en' ? I18N.en : I18N.ja
  $: catalog = cfg.language === 'en' ? CATALOG_GLOBAL : CATALOG_JA
  $: currentEntry = catalog.find(m => cfg.model === m.id || cfg.model === sakuraModelName(m.id)) ?? null
  $: otherCatalog = catalog.filter(m => cfg.model !== m.id && cfg.model !== sakuraModelName(m.id))

  async function save() {
    try {
      // API キーのトリミング
      if (cfg.anthropic_api_key) cfg.anthropic_api_key = cfg.anthropic_api_key.trim()
      if (cfg.gemini_api_key) cfg.gemini_api_key = cfg.gemini_api_key.trim()
      
      await saveConfig(cfg)
    } catch (e) {
      console.error('saveConfig failed:', e)
    }
    dispatch('saved', cfg)
    onClose()
  }
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="settings" onmousedown={(e) => e.stopPropagation()}>
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
        <option value={4}>{t.freqDev}</option>
      </select>
    </label>

    <div class="section-title">{t.llmSection}</div>

    <label>
      {t.backend}
      <select bind:value={cfg.llm_backend}>
        <option value="ollama">{t.backendOllama}</option>
        <option value="claude">{t.backendClaude}</option>
        <option value="gemini">{t.backendGemini}</option>
        <option value="router">{t.backendRouter}</option>
      </select>
    </label>

    {#if cfg.llm_backend === 'ollama' || cfg.llm_backend === 'router'}
      <label>Ollama Endpoint
        <input type="text" bind:value={cfg.ollama_endpoint} />
      </label>
      {#if !ollamaConnected}
        <div class="ollama-warn">
          {t.ollamaWarning}
          <button class="retry-btn" onclick={refreshModels}>{t.ollamaRetry}</button>
        </div>
      {/if}
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
            <!-- svelte-ignore a11y_click_events_have_key_events -->
            <!-- svelte-ignore a11y_no_static_element_interactions -->
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

    {#if cfg.llm_backend === 'gemini' || cfg.llm_backend === 'router'}
      <label>
        Gemini API Key
        <input type="password" bind:value={cfg.gemini_api_key} placeholder="AIza..." />
      </label>
    {/if}

    <label>
      {cfg.language === 'ja' ? '天気の場所' : 'Weather Location'}
      <input type="text" bind:value={cfg.weather_location} placeholder={cfg.language === 'ja' ? 'Maebashi（空欄=自動検出）' : 'Maebashi (empty = auto-detect)'} />
    </label>

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

    <!-- β(BETA) セクション -->
    <div class="section-title beta-title">{t.betaSection}</div>
    <div class="beta-content">
      <label>
        {t.tone}
        <select bind:value={cfg.tone}>
          <option value="genki">{t.toneGenki}</option>
          <option value="calm">{t.toneCalm}</option>
          <option value="oneesan">{t.toneOneesan}</option>
          <option value="tsundere">{t.toneTsundere}</option>
        </select>
      </label>

      {#if cfg.language === 'ja'}
      <label>
        {t.dialect}
        <select bind:value={cfg.dialect}>
          <option value="">{t.dialectStandard}</option>
          <option value="hakata">{t.dialectHakata} 🐟</option>
          <option value="kyoto">{t.dialectKyoto} 🌸</option>
          <option value="kansai">{t.dialectKansai} 🎭</option>
        </select>
      </label>
      {/if}
    </div>
  </div>

  <!-- ニュースフィード設定 -->
  <div class="section">
    <!-- svelte-ignore a11y_click_events_have_key_events -->
    <!-- svelte-ignore a11y_no_static_element_interactions -->
    <div class="section-toggle" onclick={() => newsOpen = !newsOpen}>
      <span class="section-title">{t.newsSection}</span>
      <span class="toggle-arrow">{newsOpen ? '▲' : '▼'}</span>
    </div>
    {#if newsOpen}
      {#each NEWS_CATEGORIES as cat}
        {@const feeds = currentFeeds(cat)}
        {@const customized = isCustomized(cat)}
        <div class="feed-category">
          <div class="feed-cat-header">
            <span class="feed-cat-label">{t.newsCategories[cat]}</span>
            {#if customized}
              <!-- svelte-ignore a11y_consider_explicit_label -->
              <button class="feed-reset-btn" onclick={() => resetFeeds(cat)}>{t.newsReset}</button>
            {/if}
          </div>
          {#each feeds as url}
            <div class="feed-item">
              <span class="feed-url">{url}</span>
              <!-- svelte-ignore a11y_consider_explicit_label -->
              <button class="feed-del-btn" onclick={() => removeFeed(cat, url)}>×</button>
            </div>
          {/each}
          <div class="feed-add-row">
            <input
              class="feed-input"
              type="text"
              placeholder={t.newsAddPlaceholder}
              bind:value={newsInputs[cat]}
              onkeydown={(e) => e.key === 'Enter' && addFeed(cat)}
            />
            <!-- svelte-ignore a11y_consider_explicit_label -->
            <button class="feed-add-btn" onclick={() => addFeed(cat)}>+</button>
          </div>
        </div>
      {/each}
    {/if}
  </div>

  <div class="buttons">
    <button class="review-btn" onclick={() => { onClose(); onOpenReview() }}>セリフレビュー</button>
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

  .beta-title {
    color: #777;
    border-top: 1px dashed #ddd;
    padding-top: 8px;
    margin-top: 12px;
  }

  .beta-content {
    background: rgba(0, 0, 0, 0.02);
    border-radius: 6px;
    padding: 6px;
    margin-bottom: 8px;
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

  .buttons {
    display: flex;
    gap: 6px;
    margin-top: 8px;
  }

  .review-btn {
    flex: 0 0 auto;
    padding: 5px 10px;
    background: #3a3a50;
    color: #a0a0c0;
    border: 1px solid #555568;
    border-radius: 6px;
    cursor: pointer;
    font-size: 10px;
  }
  .review-btn:hover { background: #4a4a60; color: #e0e0f0; }

  .save-btn {
    flex: 1;
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

  .ollama-warn { display: flex; align-items: center; justify-content: space-between; background: #fff8e1; border: 1px solid #ffe082; border-radius: 5px; padding: 4px 6px; font-size: 9px; color: #f57f17; margin-bottom: 4px; }
  .retry-btn { background: none; border: 1px solid #f57f17; color: #f57f17; border-radius: 4px; font-size: 9px; padding: 1px 6px; cursor: pointer; }
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

  .section-toggle { display: flex; justify-content: space-between; align-items: center; cursor: pointer; padding: 4px 0; }
  .section-title { font-size: 9px; color: #e91e63; text-transform: uppercase; font-weight: bold; }
  .toggle-arrow { font-size: 9px; color: #aaa; }

  .feed-category { margin-bottom: 8px; }
  .feed-cat-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 3px; }
  .feed-cat-label { font-size: 9px; font-weight: bold; color: #555; }
  .feed-reset-btn { background: none; border: none; font-size: 8px; color: #aaa; cursor: pointer; padding: 0; text-decoration: underline; }
  .feed-reset-btn:hover { color: #e91e63; }

  .feed-item { display: flex; align-items: center; gap: 4px; margin-bottom: 2px; }
  .feed-url { font-size: 8px; color: #666; flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .feed-del-btn { background: none; border: none; color: #ccc; cursor: pointer; font-size: 11px; padding: 0 2px; flex-shrink: 0; }
  .feed-del-btn:hover { color: #e91e63; }

  .feed-add-row { display: flex; gap: 3px; margin-top: 3px; }
  .feed-input { flex: 1; font-size: 8px; padding: 2px 4px; border: 1px solid #ddd; border-radius: 3px; min-width: 0; }
  .feed-add-btn { background: #e91e63; color: white; border: none; border-radius: 3px; font-size: 11px; font-weight: bold; padding: 2px 6px; cursor: pointer; }
</style>
