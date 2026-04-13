<script lang="ts">
  import { onMount, onDestroy } from 'svelte'
  import Chara from './components/Chara.svelte'
  import Balloon from './components/Balloon.svelte'
  import Settings from './components/Settings.svelte'
  import Onboarding from './components/Onboarding.svelte'
  import SpeechReview from './components/SpeechReview.svelte'
  import {
    defaultConfig,
    loadConfig,
    type AppConfig,
    onCharaClick,
    answerQuestion,
    handleQuestionAnswer,
    DetectSetupStatus,
    ExpandForOnboarding,
    onOpenSettings,
    recordNewsInterest,
    SetInteractiveMode,
  } from './lib/wails'

  const CLICK_COOLDOWN_MS = 5000
  let talkCooldown = $state(false)
  let talkCooldownTimer: ReturnType<typeof setTimeout> | null = null

  let showChatInput = $state(false)
  let chatText = $state('')
  let chatInputEl: HTMLInputElement | null = $state(null)

  const I18N_APP = {
    ja: { talkBtn: '話して', chatBtn: '話しかける', chatPlaceholder: 'さくらに話しかける...', chatSend: '→' },
    en: { talkBtn: 'Speak', chatBtn: 'Chat', chatPlaceholder: 'Say something...', chatSend: '→' },
  }

  let appStatus = $state('Idle')
  let appMood = $state('Calm')
  let speechMessage = $state({ id: 0, text: '' })
  let speechSeq = 0
  let usingFallback = $state(false)
  let isTalking = $state(false)
  let showSettings = $state(false)
  let showOnboarding = $state(false)
  let showReview = $state(false)
  let isHoveringSettings = $state(false)
  let socket: WebSocket | null = null
  let reconnectDelay = 1000
  let heartbeatTimer: ReturnType<typeof setInterval> | null = null

  let currentQuestion = $state(null as any)
  let currentNewsContext = $state('')
  let currentNewsTags = $state([] as string[])
  let newsFeedbackDone = $state(false)
  let newsFeedbackTimer: ReturnType<typeof setTimeout> | null = null

  let cfg: AppConfig = $state({ ...defaultConfig })
  const ta = $derived(cfg.language === 'en' ? I18N_APP.en : I18N_APP.ja)

  const refreshConfig = async () => {
    try {
      const loaded = await loadConfig()
      cfg = { ...cfg, ...loaded }
    } catch (err) {
      console.error('Failed to load config', err)
    }
  }

  const closeSettings = async () => {
    showSettings = false
    await refreshConfig()
  }

  function updateUI(e: any) {
    if (e.type === "question_event") {
      currentQuestion = e.payload
      isTalking = true
      return
    }
    appStatus = e.state
    appMood = e.mood
    if (e.speech) {
      usingFallback = e.using_fallback
      if (!currentQuestion) {
        speechMessage = { id: ++speechSeq, text: e.speech }
      }
      if (e.profile) {
        cfg.name = e.profile.name
        cfg.tone = e.profile.tone
      }
      if (e.news_context) {
        currentNewsContext = e.news_context
        currentNewsTags = e.news_tags ?? []
        newsFeedbackDone = false
        if (newsFeedbackTimer) clearTimeout(newsFeedbackTimer)
        newsFeedbackTimer = setTimeout(() => { currentNewsContext = '' }, 15000)
      } else {
        currentNewsContext = ''
        currentNewsTags = []
      }
    }
  }

  function handleNewsFeedback(liked: boolean) {
    if (!currentNewsContext || newsFeedbackDone) return
    newsFeedbackDone = true
    recordNewsInterest(currentNewsTags, liked)
    if (newsFeedbackTimer) {
      clearTimeout(newsFeedbackTimer)
      newsFeedbackTimer = null
    }
    // フィードバック後はすぐに消す
    currentNewsContext = ''
  }

  function connectWebSocket() {
    socket = new WebSocket('ws://localhost:34567/')
    socket.onmessage = (event) => {
      try {
        const data = JSON.parse(event.data)
        if (data.state) updateUI(data)
        else if (data.question) updateUI({ type: "question_event", payload: data })
      } catch (err) {
        console.error('Failed to parse WS message', err)
      }
    }
    socket.onopen = () => {
      reconnectDelay = 1000
      heartbeatTimer = setInterval(() => {
        if (socket?.readyState === WebSocket.OPEN) socket.send('ping')
      }, 30000)
    }
    socket.onclose = scheduleReconnect
    socket.onerror = scheduleReconnect
  }

  function scheduleReconnect() {
    cleanupSocket()
    setTimeout(() => {
      reconnectDelay = Math.min(reconnectDelay * 1.5, 8000)
      connectWebSocket()
    }, reconnectDelay)
  }

  function cleanupSocket() {
    if (heartbeatTimer) { clearInterval(heartbeatTimer); heartbeatTimer = null }
    if (socket) {
      socket.onclose = null; socket.onerror = null; socket.close(); socket = null
    }
  }

  function handleTalkClick() {
    if (showOnboarding || talkCooldown) return
    talkCooldown = true
    onCharaClick()
    if (talkCooldownTimer) clearTimeout(talkCooldownTimer)
    talkCooldownTimer = setTimeout(() => { talkCooldown = false }, CLICK_COOLDOWN_MS)
  }

  function handleChatToggle() {
    showChatInput = !showChatInput
    chatText = ''
    if (showChatInput) setTimeout(() => chatInputEl?.focus(), 50)
  }

  function handleChatSubmit() {
    const text = chatText.trim()
    if (!text) return
    answerQuestion(text)
    chatText = ''; showChatInput = false
  }

  function handleAnswer(traitID: string, index: number, text: string) {
    handleQuestionAnswer(traitID, index, text)
    currentQuestion = null
    speechMessage = { id: ++speechSeq, text: '' }
  }

  // インタラクティブモードの同期
  function reportInteractiveRegions() {
    // @ts-ignore
    const app = window.go?.main?.App
    if (!app) return
    const rects: number[] = []
    // action-bar は常に対象
    const actionBar = document.querySelector('.action-bar')
    if (actionBar) {
      const r = actionBar.getBoundingClientRect()
      if (r.width > 0) rects.push(r.left, r.top, r.width, r.height)
    }
    // バルーン（表示中のみ）
    if (isTalking || currentQuestion) {
      const balloon = document.querySelector('.balloon-positioner')
      if (balloon) {
        const r = balloon.getBoundingClientRect()
        if (r.width > 0) rects.push(r.left, r.top, r.width, r.height)
      }
    }
    // ニュースフィードバック（表示中のみ）
    if (currentNewsContext && !newsFeedbackDone) {
      const nf = document.querySelector('.news-feedback')
      if (nf) {
        const r = nf.getBoundingClientRect()
        if (r.width > 0) rects.push(r.left, r.top, r.width, r.height)
      }
    }
    app.UpdateInteractiveRegions(rects)
  }

  // バルーンが閉じたとき currentQuestion が残っていたらクリアする。
  // 質問の30秒タイムアウト時に visible=false→isTalking=false となるが、
  // currentQuestion は Balloon 側からクリアされないため、以降の発話が表示されなくなるバグを防ぐ。
  $effect(() => {
    if (!isTalking && currentQuestion) {
      currentQuestion = null
      speechMessage = { id: ++speechSeq, text: '' }
    }
  })

  $effect(() => {
    const fullInteractiveRequired = showSettings || showOnboarding || showChatInput || showReview
    SetInteractiveMode(fullInteractiveRequired)
    if (!fullInteractiveRequired) {
      // 依存する変数を参照して reactive に追跡させる
      void isTalking; void currentQuestion; void currentNewsContext; void newsFeedbackDone
      reportInteractiveRegions()
    }
  })

  onMount(async () => {
    await refreshConfig()
    // 初回レイアウト確定後に interactive regions を報告
    requestAnimationFrame(() => reportInteractiveRegions())

    if ((window as any).runtime) {
      const r = (window as any).runtime
      r.EventsOn('monitor_event', updateUI)
      r.EventsOn('observation_event', updateUI)
      r.EventsOn('greeting_event', updateUI)
      r.EventsOn('click_event', updateUI)
      r.EventsOn('question_reply_event', updateUI)
      r.EventsOn('question_event', (payload: any) => updateUI({ type: "question_event", payload }))
    } else {
      connectWebSocket()
    }

    onOpenSettings(() => { showSettings = true })

    // Shift+R でレビューパネルを開く
    document.addEventListener('keydown', (e: KeyboardEvent) => {
      if (e.shiftKey && e.key === 'R' && !showSettings && !showOnboarding) {
        showReview = !showReview
      }
    })

    const status = await DetectSetupStatus()
    if (status.is_first_run) {
      showOnboarding = true
      await ExpandForOnboarding()
    }
  })

  onDestroy(() => cleanupSocket())

  const isRightSide = $derived(cfg.window_position?.endsWith('right'))
  const isTopSide = $derived(cfg.window_position?.startsWith('top'))
  $effect(() => {
    if (!isTalking && (appMood === 'StrongJoy' || appMood === 'Positive')) {
      appMood = 'Neutral'
    }
  })
  const displayedMood = $derived(appMood || 'Neutral')
</script>

<main>
  <div class="chara-container" class:pos-top={isTopSide} class:pos-bottom={!isTopSide}>
    <div class="balloon-column">
      <div class="balloon-positioner">
        <Balloon
          bind:visible={isTalking}
          message={speechMessage}
          scale={cfg.scale}
          {usingFallback}
          position={cfg.window_position}
          language={cfg.language}
          question={currentQuestion}
          onanswer={handleAnswer}
        />
      </div>
      {#if currentNewsContext && !newsFeedbackDone}
        <div class="news-feedback">
          <span class="news-feedback-label">興味ある？</span>
          <button class="news-btn like" onclick={() => handleNewsFeedback(true)}>👍</button>
          <button class="news-btn dislike" onclick={() => handleNewsFeedback(false)}>👎</button>
        </div>
      {/if}
    </div>

    <div class="chara-column">
      <div class="chara-flip-wrapper" style="transform: scaleX({isRightSide ? -1 : 1})">
        <Chara status={appStatus} mood={displayedMood} scale={cfg.scale} isTalking={isTalking} />
      </div>
      <div class="action-bar">
        {#if showChatInput}
          <div class="chat-input-row">
            <input bind:this={chatInputEl} class="chat-input" type="text" placeholder={ta.chatPlaceholder} bind:value={chatText}
              onkeydown={(e) => { if (e.key === 'Enter') handleChatSubmit(); if (e.key === 'Escape') showChatInput = false }} />
            <button class="chat-send-btn" onclick={handleChatSubmit} disabled={!chatText.trim()}>{ta.chatSend}</button>
          </div>
        {:else}
          <button class="action-btn talk-btn" class:cooldown={talkCooldown} disabled={talkCooldown} onclick={handleTalkClick}>{ta.talkBtn}</button>
          <button class="action-btn chat-btn" onclick={handleChatToggle}>{ta.chatBtn}</button>
        {/if}
      </div>
    </div>
  </div>

  {#if showOnboarding}
    <div class="modal-backdrop onboarding-backdrop" role="presentation" onkeydown={() => {}}>
      <Onboarding onClose={() => showOnboarding = false} oncompleted={refreshConfig} currentSpeech={speechMessage.text} />
    </div>
  {:else if showSettings}
    <div class="modal-backdrop settings-backdrop" role="presentation" onkeydown={(e) => e.stopPropagation()} onclick={(e) => e.target === e.currentTarget && closeSettings()}>
      <div class="settings-content-wrapper" role="presentation" onkeydown={(e) => e.stopPropagation()} onclick={(e) => e.stopPropagation()}>
        <Settings onClose={closeSettings} on:saved={refreshConfig} onOpenReview={() => { closeSettings(); showReview = true }} />
      </div>
    </div>
  {/if}

  {#if showReview}
    <SpeechReview onClose={() => showReview = false} />
  {/if}
</main>

<style>
  :global(html), :global(body) {
    margin: 0; padding: 0; width: 100%; height: 100%;
    background: transparent !important; overflow: hidden;
    font-family: sans-serif;
    pointer-events: none; /* 背景は透過 */
  }
  main {
    width: 100vw; height: 100vh; position: relative;
    background: transparent !important;
    pointer-events: none;
  }
  .chara-container {
    position: absolute; display: flex; width: 100%; height: 100%;
    padding: 15px; justify-content: flex-end; box-sizing: border-box;
    pointer-events: none;
  }
  .pos-top { align-items: flex-start; }
  .pos-bottom { align-items: flex-end; }
  .balloon-column {
    display: flex; flex-direction: column; align-items: flex-end;
    margin-right: -25px; margin-top: 10px; pointer-events: none;
  }
  .chara-column {
    display: flex; flex-direction: column; align-items: center;
    z-index: 20; pointer-events: none;
  }
  .chara-flip-wrapper { pointer-events: none; }
  .action-bar {
    display: flex; gap: 5px; margin-top: -4px; align-items: center;
    pointer-events: auto; /* ボタンは実体 */
    opacity: 0.4; transition: opacity 0.2s;
  }
  .action-bar:hover { opacity: 1; }
  .balloon-positioner, .news-feedback, .action-btn, .chat-input-row {
    pointer-events: auto; /* これらは実体 */
  }
  .action-btn {
    background: rgba(255, 255, 255, 0.9); border: 1px solid #ccc;
    border-radius: 12px; font-size: 10px; padding: 3px 10px; cursor: pointer;
  }
  .chat-input-row {
    background: white; border-radius: 14px; padding: 3px 10px;
    display: flex; gap: 4px; border: 1px solid #e91e63;
  }
  .chat-input { border: none; outline: none; font-size: 11px; width: 140px; }
  .modal-backdrop {
    position: fixed; top: 0; left: 0; width: 100%; height: 100%;
    display: flex; align-items: center; justify-content: center;
    background: rgba(0, 0, 0, 0.2); z-index: 1000; pointer-events: auto;
  }
  .news-feedback {
    background: rgba(255, 255, 255, 0.9);
    border: 1px solid #e91e63;
    border-radius: 10px;
    padding: 2px 8px;
    display: flex;
    align-items: center;
    gap: 4px;
    margin-top: 4px;
    box-shadow: 0 2px 8px rgba(0, 0, 0, 0.1);
  }
  .news-feedback-label {
    font-size: 9px;
    color: #e91e63;
    font-weight: bold;
  }
  .news-btn {
    background: none;
    border: none;
    cursor: pointer;
    font-size: 12px;
    padding: 0 2px;
    transition: transform 0.1s;
  }
  .news-btn:hover { transform: scale(1.2); }
</style>
