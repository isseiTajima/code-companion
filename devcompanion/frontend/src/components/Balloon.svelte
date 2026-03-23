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
  const maxLength = 40

  let showInstallButton = $state(false)
  let displayText = $state('')
  let showFreeInput = $state(false)
  let freeInputText = $state('')

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
      timer = setTimeout(() => { visible = false }, 4000)
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
      showFreeInput = false
      freeInputText = ''
      onanswer(question.trait_id, index, text)
      visible = false
    }
  }

  function handleFreeSubmit() {
    const text = freeInputText.trim()
    if (text) handleAnswer(-1, text)
  }

  function handleSkip() {
    handleAnswer(-1, '対象なし')
  }

  // 質問が変わったら自由入力をリセット
  $effect(() => {
    if (!question) {
      showFreeInput = false
      freeInputText = ''
    }
  })

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

      {#if usingFallback}
        <span class="fallback-label">🔄</span>
      {/if}

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
          {#if showFreeInput}
            <div class="free-input-row">
              <!-- svelte-ignore a11y_autofocus -->
              <input
                class="free-input"
                type="text"
                placeholder="自由に入力..."
                bind:value={freeInputText}
                autofocus
                onkeydown={(e) => e.key === 'Enter' && handleFreeSubmit()}
              />
              <button class="free-submit-btn" onclick={handleFreeSubmit} disabled={!freeInputText.trim()}>→</button>
            </div>
          {:else}
            <button class="free-btn" onclick={() => showFreeInput = true}>✏️ 自由入力</button>
          {/if}
          <button class="skip-btn" onclick={handleSkip}>対象なし</button>
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

  .free-btn {
    background: none;
    border: 1px dashed #ccc;
    border-radius: 8px;
    padding: 4px 10px;
    font-size: 9px;
    color: #999;
    cursor: pointer;
    text-align: left;
    transition: border-color 0.2s, color 0.2s;
  }

  .free-btn:hover {
    border-color: #c2185b;
    color: #c2185b;
  }

  .skip-btn {
    background: none;
    border: none;
    padding: 2px 0;
    font-size: 9px;
    color: #bbb;
    cursor: pointer;
    text-align: left;
    text-decoration: underline;
  }

  .skip-btn:hover {
    color: #888;
  }

  .free-input-row {
    display: flex;
    gap: 4px;
    align-items: center;
  }

  .free-input {
    flex: 1;
    font-size: 10px;
    border: 1px solid #f8bbd0;
    border-radius: 6px;
    padding: 4px 6px;
    outline: none;
    min-width: 0;
  }

  .free-input:focus {
    border-color: #e91e63;
  }

  .free-submit-btn {
    background: #e91e63;
    color: white;
    border: none;
    border-radius: 5px;
    padding: 4px 7px;
    font-size: 10px;
    cursor: pointer;
    flex-shrink: 0;
  }

  .free-submit-btn:disabled {
    background: #ccc;
    cursor: default;
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
