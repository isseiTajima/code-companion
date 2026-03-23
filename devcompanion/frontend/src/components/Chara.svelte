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
    // 特殊表情中や作業中はまばたきを止める
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
      if (joyFrameTimer) clearInterval(joyFrameTimer)
      if (sadTimer) clearInterval(sadTimer)
      if (pcTimer) clearInterval(pcTimer)
    }
  })

  import { onMount } from 'svelte'

  // 状態に応じた画像選択
  const currentImg = $derived.by(() => {
    // PC作業（集中）
    if (mood === 'Focus') {
      return pcFrame === 0 ? c4PC1 : c4PC2
    }

    // 悲しみ状態
    if (mood === 'Sadness' || mood === 'Negative') {
      return sadFrame === 0 ? c3Sad1 : c3Sad2
    }

    // 喜び状態
    if (mood === 'StrongJoy' || mood === 'Positive') {
      return joyFrame === 0 ? c2Happy1 : c2Happy2
    }
    
    // 通常状態（瞬き）
    if (eyeState === 0) return c1Open
    if (eyeState === 1) return c1Half
    return c1Close
  })
</script>

<button
  class="chara-button"
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
</button>

<style>
  .chara-button {
    display: inline-block;
    line-height: 0;
    width: auto;
    height: auto;
    animation: floating 3s ease-in-out infinite;
    cursor: pointer;
    transition: transform 0.2s, opacity 0.5s;
    opacity: 0.8;
    background: none;
    border: none;
    padding: 0;
  }

  .chara-button.flipped {
    transform: scaleX(-1) !important;
  }
  .chara-button.flipped img {
    transform: scaleX(1);
  }

  .chara-button.talking {
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
