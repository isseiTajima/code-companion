<script lang="ts">
  import c1Open from '../assets/pixel-chara1-open.png'
  import c1Half from '../assets/pixel-chara1-half.png'
  import c1Close from '../assets/pixel-chara1-close.png'

  import c2Happy1 from '../assets/pixel-chara2.png'
  import c2Happy2 from '../assets/pixel-chara2-happy.png'

  import c3Sad1 from '../assets/pixel-chara3.png'
  import c3Sad2 from '../assets/pixel-chara3-2.png'

  import c4PC1 from '../assets/pixel-chara4-pc1.png'
  import c4PC2 from '../assets/pixel-chara4-pc2.png'

  import { onMount, onDestroy } from 'svelte'

  let {
    status = 'Idle',
    mood = 'Neutral',
    scale = 1,
    isTalking = false,
  } = $props()

  // 目パチの状態管理
  let eyeState = $state(0) // 0:open, 1:half, 2:close
  let blinkTimer: ReturnType<typeof setTimeout>

  // 喜びアニメーション用
  let joyFrame = $state(0)
  let joyFrameTimer: ReturnType<typeof setInterval> | null = null

  // 悲しみの固有アニメーション用
  let sadFrame = $state(0)
  let sadTimer: ReturnType<typeof setInterval> | null = null

  // PC作業（集中）のアニメーション用
  let pcFrame = $state(0)
  let pcTimer: ReturnType<typeof setInterval> | null = null

  $effect(() => {
    // 喜び: mood prop が直接示している間だけアニメーション
    if (mood === 'StrongJoy' || mood === 'Positive') {
      if (!joyFrameTimer) {
        joyFrameTimer = setInterval(() => { joyFrame = (joyFrame + 1) % 2 }, 500)
      }
    } else {
      if (joyFrameTimer) { clearInterval(joyFrameTimer); joyFrameTimer = null }
      joyFrame = 0
    }

    // 悲しみ
    if (mood === 'Sadness' || mood === 'Negative') {
      if (!sadTimer) {
        sadTimer = setInterval(() => { sadFrame = (sadFrame + 1) % 2 }, 500)
      }
    } else {
      if (sadTimer) { clearInterval(sadTimer); sadTimer = null }
      sadFrame = 0
    }

    // PC作業（集中）
    if (mood === 'Focus') {
      if (!pcTimer) {
        pcTimer = setInterval(() => { pcFrame = (pcFrame + 1) % 2 }, 500)
      }
    } else {
      if (pcTimer) { clearInterval(pcTimer); pcTimer = null }
      pcFrame = 0
    }
  })

  function blink() {
    if (mood === 'Sadness' || mood === 'Negative' || mood === 'StrongJoy' || mood === 'Positive' || mood === 'Focus') {
      blinkTimer = setTimeout(blink, 1000)
      return
    }
    eyeState = 1
    setTimeout(() => {
      eyeState = 2
      setTimeout(() => {
        eyeState = 1
        setTimeout(() => {
          eyeState = 0
          const nextBlink = 2000 + Math.random() * 4000
          blinkTimer = setTimeout(blink, nextBlink)
        }, 80)
      }, 100)
    }, 80)
  }

  onMount(() => {
    blinkTimer = setTimeout(blink, 3000)
  })

  onDestroy(() => {
    clearTimeout(blinkTimer)
    if (joyFrameTimer) clearInterval(joyFrameTimer)
    if (sadTimer) clearInterval(sadTimer)
    if (pcTimer) clearInterval(pcTimer)
  })

  // 状態に応じた画像選択
  const currentImg = $derived.by(() => {
    if (mood === 'Focus') return pcFrame === 0 ? c4PC1 : c4PC2
    if (mood === 'Sadness' || mood === 'Negative') return sadFrame === 0 ? c3Sad1 : c3Sad2
    if (mood === 'StrongJoy' || mood === 'Positive') return joyFrame === 0 ? c2Happy1 : c2Happy2
    if (eyeState === 0) return c1Open
    if (eyeState === 1) return c1Half
    return c1Close
  })
</script>

<div class="chara-display" class:talking={isTalking}>
  <img
    src={currentImg}
    alt="Character"
    class="pixel-art"
    style="width: {Math.round(128 * scale)}px"
  />
</div>

<style>
  .chara-display {
    display: inline-block;
    line-height: 0;
    width: auto;
    height: auto;
    animation: floating 3s ease-in-out infinite;
    transition: opacity 0.3s;
    opacity: 0.8;
    /* キャラクタ画像は常にクリック透過させる */
    pointer-events: none !important;
  }

  img.pixel-art {
    height: auto;
    image-rendering: pixelated;
    /* 画像そのものも確実に透過 */
    pointer-events: none !important;
  }

  @keyframes floating {
    0%, 100% { transform: translateY(0); }
    50% { transform: translateY(-10px); }
  }
</style>
