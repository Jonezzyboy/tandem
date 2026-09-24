<script lang="ts">
  import { onMount } from 'svelte'
  import { api } from './lib/api'
  import { app, currentId, init, loadInbox, navigate } from './lib/state.svelte'
  import { actions, initSettings, prefs, shortcutFor, shortcutFromEvent } from './lib/settings.svelte'
  import Sidebar from './components/Sidebar.svelte'
  import ChangeView from './components/ChangeView.svelte'
  import InboxView from './components/InboxView.svelte'
  import NewChange from './components/NewChange.svelte'
  import SettingsView from './components/SettingsView.svelte'
  import Icon from './components/Icon.svelte'

  onMount(() => {
    initSettings()
    init()
    navigate({ name: 'inbox' })
  })

  function run(id: string) {
    switch (id) {
      case 'newChange':
        return navigate({ name: 'new' })
      case 'inbox':
        return navigate({ name: 'inbox' })
      case 'settings':
        return navigate({ name: 'settings' })
      case 'refresh': {
        const change = currentId()
        if (change) api.refresh(change)
        else loadInbox(true)
        return
      }
      default:
        // Change-scoped actions are handled by the open ChangeView.
        if (currentId()) window.dispatchEvent(new CustomEvent('tandem:command', { detail: id }))
    }
  }

  // ⌘R is always taken over so the webview never reloads and drops state.
  function onKey(e: KeyboardEvent) {
    if (prefs.recording) return
    const pressed = shortcutFromEvent(e)
    if (!pressed) return
    const action = actions.find((a) => shortcutFor(a.id) === pressed)
    if (action && (action.scope === 'app' || currentId())) {
      e.preventDefault()
      run(action.id)
      return
    }
    if (pressed === 'meta+r') {
      e.preventDefault()
      return
    }
    const jump = /^meta\+([1-9])$/.exec(pressed)
    if (jump) {
      const c = app.changes[Number(jump[1]) - 1]
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
    {:else if app.route.name === 'settings'}
      <SettingsView />
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
    border: 1px solid var(--warn-border);
    border-radius: 10px;
    color: var(--warn-text);
    font-size: 13px;
    box-shadow: 0 8px 24px var(--shadow);
  }
</style>
