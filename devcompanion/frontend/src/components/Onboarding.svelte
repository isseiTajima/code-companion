<script lang="ts">
  import { onMount, createEventDispatcher } from 'svelte'
  import {
    DetectSetupStatus,
    CompleteSetup,
    InstallOllama,
    CancelInstall,
    saveConfig,
    loadConfig,
    type AppConfig
  } from '../lib/wails'

  let { onClose = () => {}, currentSpeech = '' } = $props();
  const dispatch = createEventDispatcher()

  type Step = 'welcome' | 'ask_name' | 'detect' | 'ask_method' | 'select_model' | 'confirm_brew' | 'input_claude' | 'installing' | 'finish'
  let currentStep = $state<Step>('welcome')

  let setupStatus = $state({ is_first_run: true, detected_backends: [] as string[], has_claude_key: false, has_brew: false })
  let config = $state<AppConfig | null>(null)
  let claudeKey = $state('')
  let userName = $state('')
  // モデル選択後の次アクション（install or finish）
  let afterModelStep = $state<'install' | 'finish'>('install')

  const MODELS = [
    { id: 'qwen2.5:4b', label: 'Standard (recommended)', size: 'Qwen 2.5 4B', vram: '~3GB' },
    { id: 'qwen2.5:7b', label: 'High quality',           size: 'Qwen 2.5 7B', vram: '~5GB' },
  ]

  async function init() {
    try {
      setupStatus = await DetectSetupStatus()
      config = await loadConfig()
      if (config) userName = config.user_name
      await new Promise(r => setTimeout(r, 2000))
      currentStep = 'ask_name'
    } catch (err) {
      currentStep = 'ask_name'
    }
  }

  function nextAfterName() {
    if (config) {
      config.user_name = userName
      saveConfig(config)
    }
    if (setupStatus.detected_backends && setupStatus.detected_backends.length > 0) {
      currentStep = 'detect'
    } else {
      currentStep = 'ask_method'
    }
  }

  onMount(() => { init() })

  // WebSocketからのセリフを監視して、インストール完了を検知
  $effect(() => {
    if (currentSpeech.includes('[INSTALL_COMPLETE]')) {
      currentStep = 'finish'
    }
  })

  async function useDetected(method: string) {
    if (!config) return
    config.llm_backend = method
    if (method === 'ollama') {
      // Ollama検出済みの場合もモデルを選ばせる
      afterModelStep = 'finish'
      currentStep = 'select_model'
    } else {
      await saveConfig(config)
      currentStep = 'finish'
    }
  }

  async function selectModel(modelId: string) {
    if (!config) return
    config.model = modelId
    await saveConfig(config)
    if (afterModelStep === 'finish') {
      currentStep = 'finish'
    } else {
      // インストールへ
      requestInstall()
    }
  }

  function requestInstall() {
    if (!setupStatus.has_brew) {
      currentStep = 'confirm_brew'
    } else {
      startInstall()
    }
  }

  function goToModelSelect() {
    afterModelStep = 'install'
    currentStep = 'select_model'
  }

  async function startInstall() {
    currentStep = 'installing'
    try {
      await InstallOllama()
    } catch (err) {
      console.error('Install failed', err)
      currentStep = 'ask_method'
    }
  }

  async function saveClaudeKey() {
    if (!config) return
    config.llm_backend = 'claude'
    config.anthropic_api_key = claudeKey
    await saveConfig(config)
    currentStep = 'finish'
  }

  async function finish() {
    await CompleteSetup()
    dispatch('completed')
    onClose()
  }
</script>

<div class="onboarding-card">
  <div class="header">
    <span class="title">🌸 はじめてのセットアップ</span>
  </div>

  <div class="content">
    {#if currentStep === 'welcome'}
      <p>はじめまして！サクラです。<br/>一緒に開発を楽しみましょう！</p>
      <div class="loading-dots"><span>.</span><span>.</span><span>.</span></div>

    {:else if currentStep === 'ask_name'}
      <p>あなたのことをなんと呼べばいいですか？</p>
      <input type="text" bind:value={userName} placeholder="ご主人様" />
      <button class="primary" disabled={!userName} onclick={nextAfterName}>これで呼んでください！</button>

    {:else if currentStep === 'detect'}
      <p>あなたの環境を調べたら...<br/>
        {#if setupStatus.detected_backends.includes('ollama')}
          <b>Ollama</b> が見つかりました！
        {:else if setupStatus.detected_backends.includes('gemini')}
          <b>Gemini (ai)</b> が見つかりました！
        {/if}
        これを使って、サクラと<br/>お喋りできるようにしますか？
      </p>
      <div class="buttons">
        <button class="primary" onclick={() => useDetected(setupStatus.detected_backends[0])}>はい、お願いします！</button>
        <button class="secondary" onclick={() => currentStep = 'ask_method'}>他のを選びたいです</button>
      </div>

    {:else if currentStep === 'ask_method'}
      <p>サクラがあなたとお喋りするための<br/><b>「力の源（LLM）」</b>を選んでください！</p>
      <div class="buttons-grid">
        <button class="choice" onclick={goToModelSelect}>
          <span class="icon">🏠</span>
          <span class="label">ローカル (Ollama)</span>
          <span class="sub">無料でプライバシーも安心</span>
        </button>
        <button class="choice" onclick={() => currentStep = 'input_claude'}>
          <span class="icon">☁️</span>
          <span class="label">クラウド (Claude API)</span>
          <span class="sub">一番賢いサクラになります</span>
        </button>
        <button class="choice" onclick={() => useDetected('router')}>
          <span class="icon">✨</span>
          <span class="label">おまかせ</span>
          <span class="sub">あるものを自動で使います</span>
        </button>
      </div>

    {:else if currentStep === 'select_model'}
      <p>使うモデルを選んでください</p>
      <div class="model-list">
        {#each MODELS as m}
          <button class="model-card" onclick={() => selectModel(m.id)}>
            <span class="model-badge">🌸 {m.label}</span>
            <span class="model-name">{m.size}</span>
            <span class="model-size">{m.vram}</span>
          </button>
        {/each}
      </div>
      <button class="secondary back-btn" onclick={() => currentStep = afterModelStep === 'finish' ? 'detect' : 'ask_method'}>戻る</button>

    {:else if currentStep === 'confirm_brew'}
      <p>Homebrew が見つかりませんでした。<br/>インストールに必要なため、先に入れてもいいですか？</p>
      <div class="buttons">
        <button class="primary" onclick={startInstall}>このまま続ける（zip）</button>
        <button class="secondary" onclick={() => currentStep = 'ask_method'}>戻る</button>
      </div>
      <p style="font-size: 10px; color: #999; margin-top: 8px;">Homebrew があると安全・確実です。<br/>brew.sh から入れてから再挑戦もできます！</p>

    {:else if currentStep === 'input_claude'}
      <p>Claude の API キーを教えてください！</p>
      <input type="password" bind:value={claudeKey} placeholder="sk-ant-..." />
      <div class="buttons">
        <button class="primary" disabled={!claudeKey} onclick={saveClaudeKey}>保存する</button>
        <button class="secondary" onclick={() => currentStep = 'ask_method'}>戻る</button>
      </div>

    {:else if currentStep === 'installing'}
      <p>今、一生懸命準備しています！<br/>{currentSpeech.replace('[INSTALL_COMPLETE]', '').replace('[INSTALL_ERROR]', '') || '準備中...'}</p>
      <div class="progress-bar"><div class="fill"></div></div>
      {#if currentSpeech.includes('[INSTALL_ERROR]')}
        <div class="error-tips">
          <p><b>うまくいかない時は...</b></p>
          <ul>
            <li>Ollamaを一度終了して再起動してみてください</li>
            <li><a href="https://ollama.com" target="_blank">ollama.com</a> から手動でインストールも確実です</li>
            <li>Macなら <code>brew install --cask ollama</code> でも入ります</li>
            <li>ネットが不安定か、容量不足かもしれません</li>
          </ul>
        </div>
        <button class="primary" onclick={() => { currentStep = 'ask_method' }}>やり直す</button>
      {:else}
        <button class="cancel-btn" onclick={async () => { await CancelInstall(); currentStep = 'ask_method' }}>中止する</button>
      {/if}

    {:else if currentStep === 'finish'}
      <p>準備完了です！<br/>これからあなたの開発を全力で応援します！</p>
      <button class="primary" onclick={finish}>さっそく始める！</button>
    {/if}
  </div>
</div>

<style>
  .onboarding-card { background: white; border: 1.5px solid #eee; border-radius: 14px; padding: 14px; width: 200px; box-shadow: 0 12px 32px rgba(0, 0, 0, 0.15); color: #333; animation: slideUp 0.5s cubic-bezier(0.16, 1, 0.3, 1); }
  @keyframes slideUp { from { opacity: 0; transform: translateY(20px) scale(0.95); } to { opacity: 1; transform: translateY(0) scale(1); } }
  .header { margin-bottom: 12px; text-align: center; }
  .title { font-weight: bold; font-size: 12px; color: #e91e63; }
  p { font-size: 11px; line-height: 1.5; margin: 0 0 12px; text-align: center; }
  .buttons, .buttons-grid { display: flex; flex-direction: column; gap: 7px; }
  button { padding: 8px; border-radius: 8px; border: none; font-size: 11px; font-weight: bold; cursor: pointer; transition: all 0.2s; }
  button.primary { background: #e91e63; color: white; }
  button.secondary { background: #f5f5f5; color: #666; }
  button.choice { background: white; border: 1.5px solid #eee; display: flex; flex-direction: column; align-items: center; padding: 9px 8px; gap: 3px; }
  button.choice:hover { border-color: #e91e63; background: #fff0f5; }
  .choice .icon { font-size: 18px; }
  .choice .label { font-size: 11px; color: #333; }
  .choice .sub { font-size: 10px; color: #999; font-weight: normal; }
  input { width: 100%; padding: 8px; border-radius: 8px; border: 1.5px solid #ddd; margin-bottom: 10px; box-sizing: border-box; font-size: 11px; }
  .loading-dots span { animation: blink 1.4s infinite both; font-size: 22px; color: #e91e63; line-height: 1; }
  .loading-dots span:nth-child(2) { animation-delay: 0.2s; }
  .loading-dots span:nth-child(3) { animation-delay: 0.4s; }
  @keyframes blink { 0%, 80%, 100% { opacity: 0; } 40% { opacity: 1; } }
  .cancel-btn { background: none; border: 1px solid #ddd; color: #999; font-size: 10px; padding: 4px 10px; border-radius: 6px; margin-top: 8px; cursor: pointer; font-weight: normal; width: 100%; }
  .cancel-btn:hover { border-color: #e91e63; color: #e91e63; }
  .progress-bar { width: 100%; height: 6px; background: #f0f0f0; border-radius: 3px; overflow: hidden; margin-bottom: 4px; }
  .progress-bar .fill { width: 40%; height: 100%; background: #e91e63; animation: progress 2s infinite ease-in-out; }
  @keyframes progress { 0% { transform: translateX(-100%); } 100% { transform: translateX(250%); } }
  .error-tips { background: #fff5f5; border: 1px solid #ffcdd2; border-radius: 8px; padding: 8px; margin-bottom: 10px; text-align: left; }
  .error-tips p { margin-bottom: 4px; color: #d32f2f; font-weight: bold; text-align: left; }
  .error-tips ul { margin: 0; padding-left: 16px; font-size: 9px; color: #666; }
  .error-tips li { margin-bottom: 2px; }
  .error-tips a { color: #e91e63; text-decoration: underline; }
  /* モデル選択 */
  .model-list { display: flex; flex-direction: column; gap: 6px; margin-bottom: 8px; }
  .model-card { background: white; border: 1.5px solid #eee; border-radius: 10px; display: flex; flex-direction: column; align-items: center; padding: 10px 8px; gap: 2px; cursor: pointer; transition: all 0.2s; }
  .model-card:hover { border-color: #e91e63; background: #fff0f5; }
  .model-badge { font-size: 10px; font-weight: bold; color: #e91e63; }
  .model-name { font-size: 12px; font-weight: bold; color: #333; }
  .model-size { font-size: 10px; color: #999; }
  .back-btn { width: 100%; font-size: 10px; font-weight: normal; }
</style>
