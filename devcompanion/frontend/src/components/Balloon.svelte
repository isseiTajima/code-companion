<script lang="ts">
  import { onDestroy } from 'svelte'
  import { InstallOllama } from '../lib/wails'

  type QuestionData = {
    trait_id: string;
    preamble: string;
    question: string;
    options: string[];
  }

  let { 
    message = { id: 0, text: '' }, 
    scale = 1, 
    usingFallback = false,
    visible = $bindable(false),
    position = 'top-right',
    question = null as QuestionData | null,
    onanswer = (traitID: string, index: number, text: string) => {}
  } = $props()

  let timer: ReturnType<typeof setTimeout> | null = null
  const maxLength = 200

  let showInstallButton = $state(false)
  let displayText = $state('')

  $effect(() => {
    // 質問モードの処理
    if (question) {
      displayText = question.question
      visible = true
      clearTimeout(timer ?? undefined)
      // 質問は長めに表示（30秒）してから自動で閉じる
      timer = setTimeout(() => { 
        if (question) visible = false 
      }, 30000)
      return
    }

    const text = message.text
    if (!text) {
      visible = false
      return
    }

    if (showInstallButton && !text.includes('[INSTALL_OLLAMA]')) {
      return
    }

    clearTimeout(timer ?? undefined)
    
    if (text.includes('[INSTALL_OLLAMA]')) {
      showInstallButton = true
      displayText = text.replace('[INSTALL_OLLAMA]', '')
    } else {
      showInstallButton = false
      displayText = text
    }

    visible = true
    if (!showInstallButton) {
      const duration = Math.min(15000, 8000 + (displayText.length * 150))
      timer = setTimeout(() => { visible = false }, duration)
    }
  })

  const trimmed = $derived(trimSpeech(displayText ?? ''))

  function trimSpeech(value: string) {
    if (!value) return ''
    const chars = Array.from(value)
    if (chars.length <= maxLength) return value
    return chars.slice(0, maxLength - 3).join('') + '...'
  }

  async function handleInstall() {
    showInstallButton = false
    await InstallOllama()
  }

  function handleAnswer(index: number, text: string) {
    if (question) {
      onanswer(question.trait_id, index, text)
      visible = false
    }
  }

  onDestroy(() => { if (timer) clearTimeout(timer) })
</script>

{#if visible && (trimmed || question)}
  <div class="balloon"
    class:fallback={usingFallback}
    class:question-mode={!!question}
    style="
      font-size: {Math.round(11 * scale)}px;
      padding: {Math.round(12 * scale)}px {Math.round(16 * scale)}px;
      border-radius: {Math.round(20 * scale)}px;
    "
  >
    <div class="content">
      {#if question && question.preamble}
        <p class="preamble">{question.preamble}</p>
      {/if}
      
      <p class="balloon-text">{trimmed}</p>
      
      {#if showInstallButton}
        <button class="install-btn" onclick={handleInstall}>
          今すぐインストール
        </button>
      {/if}

      {#if question}
        <div class="options">
          {#each question.options as option, i}
            <button class="option-btn" onclick={() => handleAnswer(i, option)}>
              {option}
            </button>
          {/each}
        </div>
      {/if}
    </div>
  </div>
{/if}

<style>
  .balloon {
    position: relative;
    background: rgba(255, 255, 255, 0.95);
    backdrop-filter: blur(8px);
    -webkit-backdrop-filter: blur(8px);
    border: 1.2px solid rgba(0, 0, 0, 0.1);
    /* border-radius / padding / font-size はインラインスタイルでスケール制御 */
    width: auto;
    max-width: 250px;
    min-height: 40px;
    height: auto;
    word-break: normal;
    overflow-wrap: break-word;
    font-size: 11px;
    line-height: 1.4;
    color: #333;
    z-index: 10;
    display: flex;
    align-items: center;
    justify-content: flex-start;
    text-align: left;
    box-sizing: border-box;
    box-shadow: 0 4px 15px rgba(0, 0, 0, 0.08);
  }

  .balloon.question-mode {
    border: 1.5px solid #e91e63;
    max-width: 280px;
  }

  .content {
    display: flex;
    flex-direction: column;
    gap: 6px;
    width: 100%;
  }

  .preamble {
    font-size: 11px;
    color: #e91e63;
    font-weight: bold;
    margin: 0;
  }

  .options {
    display: flex;
    flex-direction: column;
    gap: 4px;
    margin-top: 4px;
  }

  .option-btn {
    background: #fce4ec;
    border: 1px solid #f8bbd0;
    border-radius: 8px;
    padding: 6px 10px;
    font-size: 10px;
    color: #c2185b;
    cursor: pointer;
    text-align: left;
    transition: background 0.2s;
  }

  .option-btn:hover {
    background: #f8bbd0;
  }

  .install-btn {
    background: #4a90e2;
    color: white;
    border: none;
    border-radius: 6px;
    padding: 4px 8px;
    font-size: 10px;
    font-weight: bold;
    cursor: pointer;
    transition: background 0.2s;
    align-self: flex-start;
  }

  /* しっぽスタイル */
  .balloon::before, .balloon::after {
    content: '';
    position: absolute;
    bottom: 8px;
    right: -6px;
    transform: rotate(35deg);
  }

  .balloon::before {
    border-top: 5px solid transparent;
    border-bottom: 5px solid transparent;
    border-left: 12px solid rgba(0, 0, 0, 0.1);
  }

  .balloon::after {
    right: -5px;
    border-top: 4px solid transparent;
    border-bottom: 4px solid transparent;
    border-left: 11px solid rgba(255, 255, 255, 0.95);
  }

  p {
    margin: 0;
    font-weight: 500;
  }

  .balloon.fallback {
    background: rgba(255, 248, 225, 0.95);
    border: 1.2px solid rgba(255, 193, 7, 0.3);
  }

  .fallback-label {
    font-size: 12px;
  }
</style>
