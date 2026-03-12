<script lang="ts">
  import c1Open from '../assets/pixel-chara1-open.png'
  import c1Half from '../assets/pixel-chara1-half.png'
  import c1Close from '../assets/pixel-chara1-close.png'
  
  import c2Happy from '../assets/pixel-chara2-happy.png'
  
  import c3Sad1 from '../assets/pixel-chara3.png'
  import c3Sad2 from '../assets/pixel-chara3-2.png'

  let { 
    status = 'Idle', 
    mood = 'Neutral', 
    scale = 1,
    isTalking = false,
    flipped = false,
    onClick = () => {} 
  } = $props()

  // 目パチの状態管理
  let eyeState = $state(0) // 0:open, 1:half, 2:close
  let blinkTimer: ReturnType<typeof setTimeout>

  // 強い喜びの維持用
  let joyTimer: ReturnType<typeof setTimeout> | null = null
  let forceHappy = $state(false)

  // 悲しみの固有アニメーション用
  let sadFrame = $state(0)
  let sadTimer: ReturnType<typeof setInterval> | null = null

  $effect(() => {
    // 喜びの処理
    if (mood === 'StrongJoy' || mood === 'Positive') {
      forceHappy = true
      if (joyTimer) clearTimeout(joyTimer)
      joyTimer = setTimeout(() => {
        forceHappy = false
      }, 8000) // 喜びは8秒間維持
    }

    // 悲しみの処理
    if (mood === 'Sadness' || mood === 'Negative') {
      if (!sadTimer) {
        sadTimer = setInterval(() => {
          sadFrame = (sadFrame + 1) % 2
        }, 500)
      }
    } else {
      if (sadTimer) {
        clearInterval(sadTimer)
        sadTimer = null
      }
    }
  })

  function blink() {
    if (mood === 'Sadness' || mood === 'Negative') {
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
    return () => {
      clearTimeout(blinkTimer)
      if (joyTimer) clearTimeout(joyTimer)
    }
  })

  import { onMount } from 'svelte'

  // 状態に応じた画像選択
  const currentImg = $derived.by(() => {
    // 悲しみ状態
    if (mood === 'Sadness' || mood === 'Negative') {
      return sadFrame === 0 ? c3Sad1 : c3Sad2
    }

    // 強い喜び、または維持期間中
    if (mood === 'StrongJoy' || forceHappy) {
      return c2Happy
    }
    
    // 通常状態（瞬き）
    if (eyeState === 0) return c1Open
    if (eyeState === 1) return c1Half
    return c1Close
  })
</script>

<!-- svelte-ignore a11y_click_events_have_key_events -->
<!-- svelte-ignore a11y_no_static_element_interactions -->
<div 
  class="chara-wrapper"
  class:talking={isTalking}
  class:flipped={flipped}
  onclick={onClick}
>
  <img
    src={currentImg}
    alt="Character"
    class="pixel-art"
    style="width: {Math.round(128 * scale)}px"
  />
</div>

<style>
  .chara-wrapper {
    display: inline-block;
    line-height: 0;
    width: auto;
    height: auto;
    animation: floating 3s ease-in-out infinite;
    cursor: pointer;
    transition: transform 0.2s, opacity 0.5s;
    opacity: 0.8; /* 以前より少し濃く */
  }

  .chara-wrapper.flipped {
    transform: scaleX(-1) !important;
  }
  .chara-wrapper.flipped img {
    transform: scaleX(1); 
  }

  .chara-wrapper.talking {
    opacity: 1;
  }

  img.pixel-art {
    height: auto;
    image-rendering: pixelated;
  }

  @keyframes floating {
    0%, 100% { transform: translateY(0); }
    50% { transform: translateY(-10px); }
  }
</style>
