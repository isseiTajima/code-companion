<script>
  export let speech = ''

  let visible = false
  let timer = null

  $: if (speech) {
    visible = true
    clearTimeout(timer)
    timer = setTimeout(() => {
      visible = false
    }, 3000)
  }

  $: displayText = speech.slice(0, 40)
</script>

{#if visible && displayText}
  <div class="balloon">
    <p>{displayText}</p>
  </div>
{/if}

<style>
  .balloon {
    position: absolute;
    bottom: 140px;
    left: 50%;
    transform: translateX(-50%);
    background: white;
    border: 2px solid #333;
    border-radius: 12px;
    padding: 8px 12px;
    max-width: 160px;
    word-break: break-all;
    font-size: 12px;
    line-height: 1.4;
    z-index: 10;
  }

  .balloon::after {
    content: '';
    position: absolute;
    top: 100%;
    left: 50%;
    transform: translateX(-50%);
    border: 8px solid transparent;
    border-top-color: #333;
  }

  p {
    margin: 0;
  }
</style>
