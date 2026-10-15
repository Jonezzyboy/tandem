<script lang="ts">
  import { onMount } from 'svelte'
  import { api } from '@lib/api'
  import { fail, navigate } from '@lib/state.svelte'
  import { addRoot, prefs, saveSettings, type Account } from '@lib/settings.svelte'
  import * as App from '@wailsjs/go/main/App'
  import type { SetupItem } from '@lib/types'
  import Icon from './Icon.svelte'

  let items = $state<SetupItem[] | null>(null)
  let checking = $state(false)
  // Notifications asked for and refused, so only System Settings can turn them on.
  let refused = $state(false)

  const firstRun = !prefs.settings.setupSeen
  const missing = $derived((items ?? []).filter((i) => i.required && !i.ok).length)

  async function recheck() {
    checking = true
    try {
      items = await api.setup()
      App.Account().then((a) => { prefs.account = a as unknown as Account })
    } catch (e) {
      fail(e)
    } finally {
      checking = false
    }
  }

  onMount(() => {
    recheck()
    // Fixes happen in Terminal or System Settings; coming back rechecks.
    window.addEventListener('focus', recheck)
    return () => window.removeEventListener('focus', recheck)
  })

  async function fix(it: SetupItem) {
    try {
      switch (it.fix) {
        case 'terminal':
          return await api.runSetupFix(it.id)
        case 'folder':
          if (await addRoot(true)) await recheck()
          return
        case 'notifications':
          if (refused) return await api.openNotificationSettings()
          refused = !(await api.allowNotifications()).allowed
          return await recheck()
        case 'settings':
          navigate({ name: 'settings' })
          requestAnimationFrame(() => document.getElementById(`s-${it.id}`)?.scrollIntoView())
      }
    } catch (e) {
      fail(e)
    }
  }

  function fixLabel(it: SetupItem): string {
    switch (it.fix) {
      case 'terminal':
        return it.id === 'gh' && it.detail.startsWith('Not signed in') ? 'Sign in in Terminal' : 'Install in Terminal'
      case 'folder':
        return 'Choose folder'
      case 'notifications':
        return refused ? 'Open System Settings' : 'Allow notifications'
      default:
        return 'Set up'
    }
  }

  async function finish() {
    await saveSettings({ setupSeen: true })
    navigate(prefs.settings.mode === 'tester' ? { name: 'ready' } : { name: 'inbox' })
  }
</script>

<div class="page">
  <header style="--wails-draggable: drag">
    <div class="muted mono small">{firstRun ? 'Welcome' : 'Setup'}</div>
    <h1>{firstRun ? 'Set up Tandem' : 'Setup checklist'}</h1>
    <p class="muted lead">Tandem drives git and the GitHub CLI across every clone of a change. Check that this Mac has what it needs.</p>
  </header>

  <ol class="items" aria-busy={checking}>
    {#if !items}
      <li class="item muted">Checking this Mac…</li>
    {:else}
      {#each items as it (it.id)}
        <li class="item" class:done={it.ok}>
          {#if it.ok}
            <Icon name="check" size={20} color="var(--ok)" />
          {:else if it.required}
            <Icon name="alert" size={20} color="var(--warn)" />
          {:else}
            <Icon name="clock" size={20} color="var(--muted)" />
          {/if}
          <div class="text">
            <div class="title">{it.title}{#if !it.required}<span class="tag">Optional</span>{/if}</div>
            <div class="small" class:muted={it.ok || !it.required} class:warn={!it.ok && it.required}>{it.detail}</div>
          </div>
          {#if !it.ok && it.fix}
            <button class="btn small" class:primary={it.required} onclick={() => fix(it)}>{fixLabel(it)}</button>
          {/if}
        </li>
      {/each}
    {/if}
  </ol>

  <div class="foot">
    <button class="btn" disabled={checking} onclick={recheck}><Icon name="refresh" spin={checking} />Check again</button>
    <span class="muted small grow">
      {#if missing}{missing} required {missing === 1 ? 'item needs' : 'items need'} attention. You can still look around.{:else if items}Everything Tandem needs is here.{/if}
    </span>
    <button class="btn" class:primary={!missing} onclick={finish}>{firstRun ? (missing ? 'Skip for now' : 'Start using Tandem') : 'Done'}</button>
  </div>
</div>

<style>
  .page { padding: 0 32px 40px; display: flex; flex-direction: column; gap: 24px; max-width: 760px; }
  header { padding-top: 28px; }
  h1 { margin: 4px 0 0; font-family: var(--display); font-weight: 700; font-size: 30px; }
  .lead { margin: 10px 0 0; line-height: 1.55; font-size: 14px; }
  .small { font-size: 12.5px; }
  .items { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 10px; }
  .item {
    display: flex; align-items: center; gap: 14px; padding: 14px 16px;
    background: var(--panel); border: 1px solid var(--line); border-radius: 12px;
  }
  .item.done { background: transparent; }
  .text { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 3px; line-height: 1.45; }
  .title { font-weight: 500; font-size: 14px; display: flex; align-items: center; gap: 8px; }
  .tag { font-size: 11px; font-weight: 400; color: var(--muted); border: 1px solid var(--line-2); border-radius: 999px; padding: 0 7px; }
  .foot { display: flex; align-items: center; gap: 14px; }
  .grow { flex: 1; }
</style>
