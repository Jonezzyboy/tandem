<script lang="ts">
  import { app, navigate } from '@lib/state.svelte'
  import Icon from './Icon.svelte'
  import Kbd from './Kbd.svelte'

  const reviewCount = $derived(app.inbox?.review?.length ?? 0)
</script>

<nav>
  <div class="top" style="--wails-draggable: drag"></div>

  <div class="group">
    <button class="item" class:active={app.route.name === 'inbox'} onclick={() => navigate({ name: 'inbox' })}>
      <Icon name="inbox" />
      <span class="grow">Inbox</span>
      {#if reviewCount > 0}<span class="badge">{reviewCount}</span>{/if}
    </button>
    <button class="item" class:active={app.route.name === 'new'} onclick={() => navigate({ name: 'new' })}>
      <Icon name="plus" />
      <span class="grow">New change</span>
      <Kbd keys={['⌘', 'N']} />
    </button>
  </div>

  <div class="group changes">
    <div class="eyebrow label">My changes</div>
    {#each app.changes as c, i (c.id)}
      <button
        class="change"
        class:active={app.route.name === 'change' && app.route.id === c.id}
        onclick={() => navigate({ name: 'change', id: c.id })}
      >
        <span class="row">
          <span class="mono id">{c.id}</span>
          {#if i < 9}<Kbd keys={['⌘', String(i + 1)]} />{/if}
        </span>
        <span class="title">{c.title || 'Untitled change'}</span>
        <span class="headline {c.tone}">{c.headline}</span>
      </button>
    {:else}
      <p class="empty">No changes yet. Start one to put several repos on one branch.</p>
    {/each}
  </div>
</nav>

<style>
  nav {
    width: 256px;
    flex-shrink: 0;
    height: 100%;
    background: var(--nav);
    border-right: 1px solid var(--line);
    display: flex;
    flex-direction: column;
    gap: 20px;
    padding: 0 12px 16px;
    overflow-y: auto;
  }
  /* The inset titlebar's height: room for the traffic lights, and the drag handle. */
  .top { height: 52px; flex-shrink: 0; margin: 0 -12px; }
  .group { display: flex; flex-direction: column; gap: 2px; }
  .label { padding: 0 10px 8px; }
  .item, .change {
    display: flex;
    border: 0;
    background: transparent;
    border-radius: 8px;
    text-align: left;
    cursor: pointer;
    color: var(--text);
  }
  .item { align-items: center; gap: 10px; min-height: 36px; padding: 0 10px; color: var(--text-2); }
  .item:hover, .change:hover { background: var(--panel); }
  .item.active, .change.active { background: var(--selected); color: var(--text); }
  .grow { flex: 1; }
  .badge {
    font-family: var(--mono);
    font-size: 12px;
    background: var(--accent);
    color: #fff;
    border-radius: 10px;
    padding: 0 7px;
  }
  .change { flex-direction: column; gap: 2px; padding: 9px 10px; }
  .row { display: flex; justify-content: space-between; align-items: center; }
  .id { font-size: 12px; color: var(--accent-text); }
  .title { font-size: 14px; line-height: 1.3; }
  .headline { font-size: 12px; color: var(--muted); }
  .headline.warn { color: var(--warn-text); }
  .headline.ok { color: var(--ok-text); }
  .empty { margin: 0; padding: 0 10px; font-size: 13px; color: var(--muted); line-height: 1.5; }
</style>
