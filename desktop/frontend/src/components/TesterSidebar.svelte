<script lang="ts">
  import { app, navigate } from '@lib/state.svelte'
  import { keycaps, prefs, shortcutFor } from '@lib/settings.svelte'
  import { finish, tester } from '@lib/tester.svelte'
  import Icon from './Icon.svelte'
  import Avatar from './Avatar.svelte'

  const current = $derived(tester.state.current)
  const behind = $derived(tester.status.reduce((n, s) => n + s.behind, 0))
  const readyCount = $derived(tester.queue?.items.length ?? 0)

  async function backToMain() {
    if (await finish('')) navigate({ name: 'ready' })
  }
</script>

<nav>
  <div class="top" style="--wails-draggable: drag"></div>

  <div class="group">
    <button class="item" class:active={app.route.name === 'ready'} onclick={() => navigate({ name: 'ready' })}>
      <Icon name="check" />
      <span class="grow">Ready to test</span>
      {#if readyCount > 0}<span class="badge">{readyCount}</span>{/if}
    </button>
    <button class="item" class:active={app.route.name === 'recent'} onclick={() => navigate({ name: 'recent' })}>
      <Icon name="clock" />
      <span class="grow">Recently tested</span>
    </button>
  </div>

  <div class="group">
    <div class="eyebrow label">Testing now</div>
    {#if current}
      <div class="now" class:active={app.route.name === 'test' && app.route.id === current.key}>
        <button class="now-open" onclick={() => navigate({ name: 'test', id: current.key })}>
          <span class="now-id"><span class="dot"></span><span class="mono">{current.key}</span></span>
          {#if current.title}<span class="now-title">{current.title}</span>{/if}
          {#if behind > 0}
            <span class="small warn">{behind} new commit{behind === 1 ? '' : 's'} to pull</span>
          {:else}
            <span class="small ok-text">{current.repos.length} repo{current.repos.length === 1 ? '' : 's'} on {current.key}</span>
          {/if}
        </button>
        <button class="btn small back" disabled={tester.finishing} onclick={backToMain}>
          <Icon name="refresh" size={14} spin={tester.finishing} />Back to main
        </button>
      </div>
    {:else}
      <div class="idle">
        <span class="dot"></span>
        <span>Nothing checked out. Your repos are on their main branches.</span>
      </div>
    {/if}
  </div>

  <div class="account">
    <button class="who" onclick={() => navigate({ name: 'settings' })} aria-label="Account and settings">
      <Avatar account={prefs.account} size={28} />
      <span class="who-text">
        {#if prefs.account && !prefs.account.error}
          <span class="who-name">{prefs.account.name || prefs.account.login}</span>
          <span class="who-login mono"><span class="mode">Tester</span> · @{prefs.account.login}</span>
        {:else}
          <span class="who-name">Not signed in</span>
          <span class="who-login"><span class="mode mono">Tester</span> · <span class="warn">{prefs.account?.error ? 'run gh auth login' : 'checking gh…'}</span></span>
        {/if}
      </span>
    </button>
    <button class="icon-btn gear" class:active={app.route.name === 'settings'} onclick={() => navigate({ name: 'settings' })}
      aria-label="Settings" title="Settings ({keycaps(shortcutFor('settings')).join('')})">
      <Icon name="settings" />
    </button>
  </div>
</nav>

<style>
  nav {
    width: 256px; flex-shrink: 0; height: 100%; background: var(--nav); border-right: 1px solid var(--line);
    display: flex; flex-direction: column; gap: 20px; padding: 0 12px 16px; overflow: hidden;
  }
  .top { height: 52px; flex-shrink: 0; margin: 0 -12px; }
  .group { display: flex; flex-direction: column; gap: 2px; }
  .label { padding: 0 10px 8px; }
  .item {
    display: flex; align-items: center; gap: 10px; min-height: 36px; padding: 0 10px; border: 0; border-radius: 8px;
    background: transparent; color: var(--text-2); text-align: left; cursor: pointer;
  }
  .item:hover { background: var(--panel); }
  .item.active { background: var(--selected); color: var(--text); }
  .grow { flex: 1; }
  .badge { font-family: var(--mono); font-size: 12px; background: var(--accent); color: var(--on-accent); border-radius: 10px; padding: 0 7px; }
  .small { font-size: 12px; }
  .ok-text { color: var(--ok-text); }
  .dot { width: 8px; height: 8px; border-radius: 4px; background: var(--ok); flex-shrink: 0; }
  .idle {
    display: flex; gap: 10px; align-items: flex-start; padding: 12px; border-radius: 10px; border: 1px dashed var(--line-2);
    font-size: 13px; color: var(--muted); line-height: 1.45;
  }
  .idle .dot { margin-top: 5px; }
  .now { display: flex; flex-direction: column; gap: 8px; padding: 12px; border-radius: 10px; background: var(--ok-bg); border: 1px solid var(--ok-border); }
  .now.active { box-shadow: 0 0 0 1px var(--ok-border); }
  .now-open { display: flex; flex-direction: column; gap: 4px; border: 0; background: none; padding: 0; color: var(--text); text-align: left; cursor: pointer; font: inherit; }
  .now-id { display: flex; gap: 8px; align-items: center; font-size: 12px; color: var(--ok-text); }
  .now-title { font-size: 14px; line-height: 1.3; }
  .back { justify-content: center; }
  .account {
    margin: auto -12px -16px; padding: 10px 12px 12px; display: flex; align-items: center; gap: 4px;
    border-top: 1px solid var(--line); background: var(--nav);
  }
  .who {
    flex: 1; min-width: 0; display: flex; align-items: center; gap: 10px; padding: 6px 8px;
    border: 0; border-radius: 8px; background: transparent; color: var(--text); text-align: left; cursor: pointer;
  }
  .who:hover { background: var(--panel); }
  .who-text { display: flex; flex-direction: column; min-width: 0; line-height: 1.3; }
  .who-name, .who-login { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .who-name { font-size: 13px; font-weight: 500; }
  .who-login { font-size: 11.5px; color: var(--muted); }
  .mode { color: var(--ok-text); }
  .gear { width: 34px; height: 34px; }
  .gear.active { background: var(--selected); color: var(--text); }
</style>
