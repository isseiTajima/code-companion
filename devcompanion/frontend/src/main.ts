import { mount } from 'svelte'
import App from './App.svelte'

const target = document.getElementById('app') ?? document.body

// Ensure we don't have multiple instances during HMR
target.innerHTML = ''

const app = mount(App, {
  target: target,
})

export default app
