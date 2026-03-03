<script>
  import { OnCharaClick } from '../../wailsjs/go/main/App'
  export let state = 'Idle'
  export let mood = 'Calm'

  const animClass = {
    Idle:     'anim-idle',
    Thinking: 'anim-thinking',
    Running:  'anim-running',
    Success:  'anim-success',
    Fail:     'anim-fail',
    Editing:  'anim-editing',
  }

  const moodFilter = {
    Happy:   'brightness(1.2) saturate(1.3)',
    Nervous: 'hue-rotate(180deg) saturate(0.8)',
    Focus:   'contrast(1.3) brightness(0.95)',
    Calm:    'none',
  }

  $: currentAnim = animClass[state] ?? 'anim-idle'
  $: currentFilter = moodFilter[mood] ?? 'none'
</script>

<img
  src="./assets/chara.png"
  alt="キャラクター"
  class={currentAnim}
  style="filter: {currentFilter}; cursor: pointer;"
  on:click={OnCharaClick}
/>

<style>
  img {
    width: 128px;
    height: auto;
    image-rendering: pixelated;
  }

  @keyframes float {
    0%, 100% { transform: translateY(0); }
    50%       { transform: translateY(-4px); }
  }
  @keyframes wobble {
    0%, 100% { transform: rotate(0deg); }
    25%       { transform: rotate(-3deg); }
    75%       { transform: rotate(3deg); }
  }
  @keyframes bounce {
    0%, 100% { transform: translateY(0); }
    50%       { transform: translateY(-8px); }
  }
  @keyframes jump {
    0%, 100% { transform: translateY(0); }
    40%       { transform: translateY(-20px); }
  }
  @keyframes droop {
    0%, 100% { transform: rotate(0deg); }
    50%       { transform: rotate(-15deg); }
  }
  @keyframes blink {
    0%, 100% { opacity: 1; }
    50%       { opacity: 0.3; }
  }

  .anim-idle     { animation: float    3s ease-in-out infinite; }
  .anim-thinking { animation: wobble   1s ease-in-out infinite; }
  .anim-running  { animation: bounce   0.6s ease-in-out infinite; }
  .anim-success  { animation: jump     0.8s ease-in-out; }
  .anim-fail     { animation: droop    2s ease-in-out infinite; }
  .anim-editing  { animation: blink    0.8s step-end infinite; }
</style>
