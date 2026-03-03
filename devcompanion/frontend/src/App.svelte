<script>
  import { onMount } from 'svelte'
  import Chara from './components/Chara.svelte'
  import Balloon from './components/Balloon.svelte'
  import Settings from './components/Settings.svelte'

  let state = 'Idle'
  let task = 'Plan'
  let mood = 'Calm'
  let speech = ''
  let showSettings = false

  onMount(() => {
    const socket = new WebSocket('ws://localhost:34567')
    socket.onmessage = (e) => {
      const ev = JSON.parse(e.data)
      state = ev.state
      task = ev.task
      mood = ev.mood
      speech = ev.speech
    }
  })
</script>

<main>
  <div class="container">
    {#if showSettings}
      <Settings onClose={() => (showSettings = false)} />
    {:else}
      <button class="gear" on:click={() => (showSettings = true)}>⚙️</button>
      <Balloon {speech} />
      <Chara {state} {mood} />
    {/if}
  </div>
</main>

<style>
  :global(body) {
    margin: 0;
    background: transparent;
    overflow: hidden;
    font-family: sans-serif;
  }

  main {
    width: 200px;
    height: 300px;
    display: flex;
    align-items: flex-end;
    justify-content: center;
  }

  .container {
    position: relative;
    display: flex;
    flex-direction: column;
    align-items: center;
  }

  .gear {
    position: absolute;
    top: -260px;
    right: 0;
    background: none;
    border: none;
    font-size: 16px;
    cursor: pointer;
    padding: 2px;
  }
</style>
