<script lang="ts">
  import { onMount } from 'svelte'
  import { api } from './lib/api'
  import { app, currentId, init, loadInbox, navigate } from './lib/state.svelte'
  import Sidebar from './components/Sidebar.svelte'
  import ChangeView from './components/ChangeView.svelte'
  import InboxView from './components/InboxView.svelte'
  import NewChange from './components/NewChange.svelte'
  import Icon from './components/Icon.svelte'

  onMount(() => {
    init()
    navigate({ name: 'inbox' })
  })

  // ⌘R is taken over so the webview never reloads and drops state.
  function onKey(e: KeyboardEvent) {
    if (!e.metaKey || e.altKey || e.ctrlKey) return
    if (e.key === 'n') {
      e.preventDefault()
      navigate({ name: 'new' })
    } else if (e.key === 'r') {
      e.preventDefault()
      const id = currentId()
      if (id) api.refresh(id)
      else loadInbox(true)
    } else if (e.key === '0') {
      e.preventDefault()
      navigate({ name: 'inbox' })
    } else if (/^[1-9]$/.test(e.key)) {
      const c = app.changes[Number(e.key) - 1]
      if (c) {
        e.preventDefault()
        navigate({ name: 'change', id: c.id })
      }
    }
  }
</script>

<svelte:window onkeydown={onKey} />

<div class="shell">
  <Sidebar />
  <main>
    {#if app.route.name === 'inbox'}
      <InboxView />
    {:else if app.route.name === 'new'}
      <NewChange />
    {:else}
      {#key app.route.id}
        <ChangeView id={app.route.id} />
      {/key}
    {/if}
  </main>
  {#if app.error}
    <div class="toast" role="alert">
      <Icon name="alert" color="var(--warn)" />
      <span class="selectable">{app.error}</span>
      <button class="icon-btn" aria-label="Dismiss" onclick={() => (app.error = '')}><Icon name="close" /></button>
    </div>
  {/if}
</div>

<style>
  .shell { display: flex; height: 100%; }
  main { flex: 1; min-width: 0; height: 100%; overflow: auto; }
  .toast {
    position: fixed;
    right: 20px;
    bottom: 20px;
    max-width: 520px;
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 10px 8px 10px 14px;
    background: var(--warn-bg);
    border: 1px solid #8a4a2a;
    border-radius: 10px;
    color: var(--warn-text);
    font-size: 13px;
    box-shadow: 0 8px 24px rgba(0, 0, 0, 0.4);
  }
</style>
