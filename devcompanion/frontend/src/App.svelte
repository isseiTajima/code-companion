<script lang="ts">
  import { onMount, onDestroy } from 'svelte'
  import Chara from './components/Chara.svelte'
  import Balloon from './components/Balloon.svelte'
  import Settings from './components/Settings.svelte'
  import Onboarding from './components/Onboarding.svelte'
  import {
    defaultConfig,
    loadConfig,
    type AppConfig,
    onCharaClick,
    answerQuestion,
    DetectSetupStatus,
    ExpandForOnboarding,
    SetClickThrough,
    onOpenSettings,
  } from './lib/wails'

  let appStatus = $state('Idle')
  let appMood = $state('Calm')
  let speechMessage = $state({ id: 0, text: '' })
  let speechSeq = 0
  let usingFallback = $state(false)
  let isTalking = $state(false)
  let showSettings = $state(false)
  let showOnboarding = $state(false)
  let isHoveringSettings = $state(false)
  let socket: WebSocket | null = null
  let reconnectDelay = 1000
  let heartbeatTimer: ReturnType<typeof setInterval> | null = null

  // 質問データ管理
  let currentQuestion = $state(null as any)

  let cfg: AppConfig = $state({ ...defaultConfig })

  // OSレベルのクリック透過制御
  $effect(() => {
    // 質問表示中またはモーダル表示時はクリックを有効にする
    const isModalOpen = showSettings || showOnboarding || isHoveringSettings || !!currentQuestion
    const ghostMode = !isModalOpen
    SetClickThrough(ghostMode)
    document.body.dataset.ghostMode = String(ghostMode)
  })

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
      speechMessage = { id: ++speechSeq, text: e.speech }
      usingFallback = e.using_fallback
      currentQuestion = null // 通常のセリフが来たら質問を消す
      if (e.profile) {
        cfg.name = e.profile.name
        cfg.tone = e.profile.tone
      }
    }
  }

  function connectWebSocket() {
    socket = new WebSocket('ws://localhost:34567/')
    socket.onmessage = (event) => {
      try {
        const data = JSON.parse(event.data)
        // WebSocket経由のイベント形式を統一
        if (data.state) {
           updateUI(data)
        } else if (data.question) {
           updateUI({ type: "question_event", payload: data })
        }
      } catch (err) {
        console.error('Failed to parse WS message', err)
      }
    }
    socket.onopen = () => {
      reconnectDelay = 1000
      heartbeatTimer = setInterval(() => {
        if (socket?.readyState === WebSocket.OPEN) {
          socket.send('ping')
        }
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
    if (heartbeatTimer) {
      clearInterval(heartbeatTimer)
      heartbeatTimer = null
    }
    if (socket) {
      socket.onclose = null
      socket.onerror = null
      socket.close()
      socket = null
    }
  }

  const handleCharaClick = () => {
    if (showOnboarding) return
    onCharaClick()
  }

  function handleAnswer(traitID: string, index: number, text: string) {
    answerQuestion(traitID, index, text)
    currentQuestion = null
  }

  onMount(async () => {
    await refreshConfig()

    if ((window as any).runtime) {
      console.log('Running in Wails mode')
      const r = (window as any).runtime
      r.EventsOn('monitor_event', updateUI)
      r.EventsOn('observation_event', updateUI)
      r.EventsOn('greeting_event', updateUI)
      r.EventsOn('click_event', updateUI)
      r.EventsOn('question_event', (payload: any) => updateUI({ type: "question_event", payload }))
    } else {
      console.log('Running in Browser/Server mode, connecting WebSocket')
      connectWebSocket()
    }

    onOpenSettings(() => {
      showSettings = true
    })

    const status = await DetectSetupStatus()
    if (status.is_first_run) {
      showOnboarding = true
      await ExpandForOnboarding()
    }
  })

  onDestroy(() => {
    cleanupSocket()
  })

  const isRightSide = $derived(cfg.window_position?.endsWith('right'))
  const isTopSide = $derived(cfg.window_position?.startsWith('top'))
</script>

<main>
  <div class="chara-container" class:pos-top={isTopSide} class:pos-bottom={!isTopSide}>
    <div class="balloon-positioner" style="pointer-events: {currentQuestion ? 'auto' : 'none'}">
      <Balloon
        bind:visible={isTalking} 
        message={speechMessage} 
        scale={cfg.scale} 
        {usingFallback} 
        position={cfg.window_position} 
        question={currentQuestion}
        onanswer={handleAnswer}
      />
    </div>
    
    <div class="chara-flip-wrapper" style="transform: scaleX({isRightSide ? -1 : 1}); pointer-events: {(showSettings || showOnboarding || !!currentQuestion) ? 'auto' : 'none'};">
      <Chara 
        status={appStatus} 
        mood={appMood} 
        scale={cfg.scale} 
        isTalking={isTalking}
        onClick={handleCharaClick}
      />
    </div>
  </div>

  {#if showOnboarding}
    <div class="modal-backdrop onboarding-backdrop">
      <Onboarding
        onClose={() => showOnboarding = false}
        oncompleted={refreshConfig}
        currentSpeech={speechMessage.text}
      />
    </div>
  {:else if showSettings}
    <!-- svelte-ignore a11y_click_events_have_key_events -->
    <!-- svelte-ignore a11y_no_static_element_interactions -->
    <div class="modal-backdrop settings-backdrop" onclick={(e) => { if (e.target === e.currentTarget) closeSettings() }}>
      <div class="settings-content-wrapper" onclick={(e) => e.stopPropagation()} onmousedown={(e) => e.stopPropagation()}>
        <Settings onClose={closeSettings} on:saved={refreshConfig} />
      </div>
    </div>
  {/if}
</main>

<style>
  :global(html), :global(body) {
    margin: 0;
    padding: 0;
    width: 100%;
    height: 100%;
    background: transparent !important;
    overflow: hidden;
    font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif;
    pointer-events: none;
  }

  main {
    width: 100vw;
    height: 100vh;
    position: relative;
    background: transparent !important;
    pointer-events: none;
  }

  .chara-container {
    position: absolute;
    display: flex;
    pointer-events: none;
    width: 100%;
    height: 100%;
    box-sizing: border-box;
    /* 浮遊アニメーションの可動域(10px)を考慮してパディングを追加 */
    padding: 15px; 
    justify-content: flex-end; 
    flex-direction: row; 
  }

  .pos-top { align-items: flex-start; }
  .pos-bottom { align-items: flex-end; }

  .chara-flip-wrapper {
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 20;
    transform: scaleX(-1);
  }

  .balloon-positioner {
    position: relative;
    pointer-events: none;
    z-index: 10;
    margin-right: -25px; 
    margin-top: 10px;
  }

  .modal-backdrop {
    position: fixed;
    top: 0;
    left: 0;
    width: 100%;
    height: 100%;
    display: flex;
    align-items: center;
    justify-content: center;
    background: rgba(0, 0, 0, 0.1);
    z-index: 1000;
    pointer-events: auto;
  }

  .onboarding-backdrop {
    background: transparent;
  }
</style>
