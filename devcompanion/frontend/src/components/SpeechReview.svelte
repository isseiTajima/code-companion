<script lang="ts">
  import { onMount } from 'svelte'
  import { getUnratedSpeeches, rateSpeech, expandForReview, collapseFromReview, type SpeechReviewItem } from '../lib/wails'

  let { onClose }: { onClose: () => void } = $props()

  let queue: SpeechReviewItem[] = $state([])
  let current = $state(0)
  let total = $state(0)
  let loading = $state(true)
  let done = $state(false)
  let sliderValue = $state(5)
  let comment = $state('')

  const PERSONALITY_LABEL: Record<string, string> = {
    cute:    'おとなしめ',
    genki:   '元気',
    cool:    'ツンデレ',
    oneesan: 'お姉さん',
  }

  const CATEGORY_LABEL: Record<string, string> = {
    working:     '作業中',
    achievement: '達成',
    concern:     '心配',
    struggle:    '苦戦',
    ai_pairing:  'AIペア',
    night_work:  '夜作業',
    encourage:   '応援',
  }

  onMount(async () => {
    await expandForReview()
    queue = await getUnratedSpeeches()
    total = queue.length
    loading = false
    if (total === 0) done = true
  })

  function close() {
    collapseFromReview()
    onClose()
  }

  const item = $derived(queue[current] ?? null)
  const progress = $derived(total > 0 ? `${current + 1} / ${total}` : '0 / 0')

  function sliderColor(n: number): string {
    if (n <= 3) return '#e05252'
    if (n <= 6) return '#e0a828'
    return '#52b26a'
  }

  async function rate() {
    if (!item) return
    try {
      await rateSpeech(item, sliderValue, comment.trim())
    } catch (e) {
      console.error('RateSpeech failed:', e)
    }
    next()
  }

  function next() {
    if (current + 1 >= total) {
      done = true
    } else {
      current++
    }
    sliderValue = 5
    comment = ''
  }
</script>

<div class="panel">
  <div class="header">
    <span class="title">セリフレビュー</span>
    <button class="close-btn" onclick={close}>✕</button>
  </div>

  {#if loading}
    <div class="center-msg">読み込み中…</div>
  {:else if done}
    <div class="center-msg">
      <div class="done-icon">✓</div>
      <div>レビュー完了！</div>
      <button class="close-btn-lg" onclick={close}>閉じる</button>
    </div>
  {:else if item}
    <div class="progress">{progress}</div>

    <div class="speech-box">
      <div class="speech-text">{item.speech}</div>
      <div class="badges">
        {#if item.personality}
          <span class="badge">{PERSONALITY_LABEL[item.personality] ?? item.personality}</span>
        {/if}
        {#if item.category}
          <span class="badge badge-cat">{CATEGORY_LABEL[item.category] ?? item.category}</span>
        {/if}
      </div>
    </div>

    <div class="rating-area">
      <div class="score-display" style="color: {sliderColor(sliderValue)}">{sliderValue} 点</div>
      <input
        class="slider"
        type="range"
        min="1" max="10" step="1"
        bind:value={sliderValue}
        style="--thumb-color: {sliderColor(sliderValue)}"
      />
      <div class="score-labels"><span>1</span><span>5</span><span>10</span></div>
    </div>

    <input
      class="comment-input"
      type="text"
      placeholder="コメント（任意）"
      bind:value={comment}
      onkeydown={(e) => { e.stopPropagation(); if (e.key === 'Enter') rate() }}
    />

    <div class="actions">
      <button class="skip-btn" onclick={next}>スキップ</button>
      <button class="rate-btn" onclick={rate}>この点数で評価</button>
    </div>
  {/if}
</div>

<style>
  .panel {
    position: fixed;
    inset: 0;
    overflow-y: auto;
    background: #1e1e2e;
    padding: 16px 14px;
    color: #e0e0f0;
    font-family: sans-serif;
    box-sizing: border-box;
    pointer-events: auto;
    z-index: 200;
  }

  .header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 12px;
  }

  .title {
    font-size: 13px;
    font-weight: 600;
    color: #a0a0c0;
  }

  .close-btn {
    background: none;
    border: none;
    color: #606070;
    cursor: pointer;
    font-size: 13px;
    padding: 2px 6px;
    border-radius: 4px;
  }
  .close-btn:hover { color: #e0e0f0; }

  .progress {
    font-size: 11px;
    color: #505068;
    text-align: right;
    margin-bottom: 10px;
  }

  .speech-box {
    background: #16162a;
    border-radius: 8px;
    padding: 12px;
    margin-bottom: 16px;
  }

  .speech-text {
    font-size: 16px;
    line-height: 1.6;
    color: #f0f0ff;
    font-weight: 500;
    word-break: break-all;
    margin-bottom: 8px;
  }

  .badges { display: flex; gap: 5px; flex-wrap: wrap; }
  .badge { font-size: 10px; background: #2a2a3e; color: #8888aa; padding: 2px 7px; border-radius: 10px; }
  .badge-cat { background: #1e2a3e; color: #6688aa; }

  .rating-area {
    margin-bottom: 12px;
  }

  .score-display {
    font-size: 22px;
    font-weight: 700;
    text-align: center;
    margin-bottom: 6px;
    transition: color 0.15s;
  }

  .slider {
    width: 100%;
    box-sizing: border-box;
    appearance: none;
    height: 4px;
    border-radius: 2px;
    background: #3a3a50;
    outline: none;
    cursor: pointer;
  }
  .slider::-webkit-slider-thumb {
    appearance: none;
    width: 20px;
    height: 20px;
    border-radius: 50%;
    background: var(--thumb-color, #a0a0c0);
    cursor: pointer;
    transition: background 0.15s;
  }

  .score-labels {
    display: flex;
    justify-content: space-between;
    font-size: 10px;
    color: #505068;
    margin-top: 4px;
  }

  .comment-input {
    width: 100%;
    box-sizing: border-box;
    background: #16162a;
    border: 1px solid #3a3a50;
    border-radius: 6px;
    color: #c0c0d8;
    font-size: 12px;
    padding: 7px 10px;
    outline: none;
    margin-bottom: 10px;
  }
  .comment-input::placeholder { color: #404058; }
  .comment-input:focus { border-color: #5a5a78; }

  .actions {
    display: flex;
    gap: 8px;
  }

  .skip-btn {
    background: none;
    border: 1px solid #3a3a50;
    color: #606070;
    cursor: pointer;
    font-size: 12px;
    padding: 7px 12px;
    border-radius: 6px;
    flex: 0 0 auto;
  }
  .skip-btn:hover { color: #a0a0b0; border-color: #5a5a70; }

  .rate-btn {
    flex: 1;
    background: #2e4060;
    border: 1px solid #3e5070;
    color: #a0c0e0;
    cursor: pointer;
    font-size: 12px;
    padding: 7px;
    border-radius: 6px;
    font-weight: 600;
  }
  .rate-btn:hover { background: #3e5070; color: #c0d8f0; }

  .center-msg {
    text-align: center;
    padding: 32px 0;
    color: #808090;
    font-size: 14px;
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 12px;
  }

  .done-icon { font-size: 32px; color: #52b26a; }

  .close-btn-lg {
    background: #2a2a3e;
    border: 1px solid #3a3a50;
    color: #a0a0c0;
    cursor: pointer;
    font-size: 13px;
    padding: 8px 20px;
    border-radius: 6px;
  }
  .close-btn-lg:hover { background: #3a3a50; }
</style>
