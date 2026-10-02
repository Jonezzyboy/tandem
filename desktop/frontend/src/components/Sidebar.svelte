<script lang="ts">
  import { tick } from 'svelte'
  import { api } from '@lib/api'
  import { app, checkOut, fail, navigate, switching } from '@lib/state.svelte'
  import Icon from './Icon.svelte'
  import Kbd from './Kbd.svelte'
  import Avatar from './Avatar.svelte'
  import { keycaps, prefs, shortcutFor } from '@lib/settings.svelte'

  const reviewCount = $derived(app.inbox?.review?.length ?? 0)

  let dragging = $state<string | null>(null)
  // The gap the dragged change would land in: 0 is above the first, n below the last.
  let gap = $state<number | null>(null)

  async function move(id: string, to: number) {
    const ids = app.changes.map((c) => c.id)
    const from = ids.indexOf(id)
    if (from < 0 || to < 0 || to > ids.length || to === from || to === from + 1) return
    ids.splice(from, 1)
    ids.splice(to > from ? to - 1 : to, 0, id)
    const prev = app.changes
    app.changes = ids.map((i) => prev.find((c) => c.id === i)!)
    try {
      await api.reorder(ids)
    } catch (e) {
      app.changes = prev
      fail(e)
    }
  }

  function onDragOver(e: DragEvent, i: number) {
    if (!dragging) return
    e.preventDefault()
    e.dataTransfer!.dropEffect = 'move'
    const r = (e.currentTarget as HTMLElement).getBoundingClientRect()
    gap = e.clientY < r.top + r.height / 2 ? i : i + 1
  }

  function onDrop(e: DragEvent) {
    e.preventDefault()
    if (dragging !== null && gap !== null) move(dragging, gap)
    dragging = gap = null
  }

  async function onKey(e: KeyboardEvent, id: string, i: number) {
    if (!e.altKey || (e.key !== 'ArrowUp' && e.key !== 'ArrowDown')) return
    e.preventDefault()
    await move(id, e.key === 'ArrowUp' ? i - 1 : i + 2)
    await tick()
    document.querySelector<HTMLElement>(`[data-change="${CSS.escape(id)}"]`)?.focus()
  }
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
      <Kbd keys={keycaps(shortcutFor('newChange'))} />
    </button>
  </div>

  <div class="group changes grow-list">
    <div class="eyebrow label" id="my-changes">My changes</div>
    <div class="list" role="list" aria-labelledby="my-changes">
    {#each app.changes as c, i (c.id)}
      <div class="change-wrap" role="listitem" draggable="true"
        class:dragging={dragging === c.id} class:drop-before={gap === i} class:drop-after={gap === i + 1 && i === app.changes.length - 1}
        ondragstart={(e) => { dragging = c.id; e.dataTransfer!.effectAllowed = 'move'; e.dataTransfer!.setData('text/plain', c.id) }}
        ondragover={(e) => onDragOver(e, i)} ondrop={onDrop} ondragend={() => (dragging = gap = null)}>
        <button
          class="change"
          data-change={c.id}
          class:active={app.route.name === 'change' && app.route.id === c.id}
          onclick={() => navigate({ name: 'change', id: c.id })}
          onkeydown={(e) => onKey(e, c.id, i)}
          title="Drag, or ⌥↑ ⌥↓, to reorder"
        >
          <span class="row">
            <span class="id-line">
              <span class="mono id">{c.id}</span>
              {#if c.checkedOut}<span class="live" title="Checked out in every repo" aria-label="checked out"></span>{/if}
            </span>
            {#if i < 9}<Kbd keys={['⌘', String(i + 1)]} />{/if}
          </span>
          <span class="title">{c.title || 'Untitled change'}</span>
          <span class="headline {c.tone}">{c.headline}</span>
        </button>
        {#if !c.worktrees && !c.checkedOut && c.legs > 0}
          <button class="icon-btn quick-switch" class:busy={switching[c.id]} onclick={() => checkOut(c.id)}
            aria-label="Check out {c.id} in every repo" title="Check out {c.id} in every repo" disabled={switching[c.id]}>
            <Icon name="branch" size={14} spin={switching[c.id]} />
          </button>
        {/if}
      </div>
    {:else}
      <p class="empty">No changes yet. Start one to put several repos on one branch.</p>
    {/each}
    </div>
  </div>
  <div class="account">
    <button class="who" onclick={() => navigate({ name: 'settings' })} aria-label="Account and settings">
      <Avatar account={prefs.account} size={28} />
      <span class="who-text">
        {#if prefs.account && !prefs.account.error}
          <span class="who-name">{prefs.account.name || prefs.account.login}</span>
          <span class="who-login mono">@{prefs.account.login}</span>
        {:else}
          <span class="who-name">Not signed in</span>
          <span class="who-login warn">{prefs.account?.error ? 'run gh auth login' : 'checking gh…'}</span>
        {/if}
      </span>
    </button>
    <button
      class="icon-btn gear"
      class:active={app.route.name === 'settings'}
      onclick={() => navigate({ name: 'settings' })}
      aria-label="Settings"
      title="Settings ({keycaps(shortcutFor('settings')).join('')})"
    >
      <Icon name="settings" />
    </button>
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
    overflow: hidden;
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
    color: var(--on-accent);
    border-radius: 10px;
    padding: 0 7px;
  }
  .change { flex-direction: column; gap: 2px; padding: 9px 10px; width: 100%; }
  .list { display: flex; flex-direction: column; gap: 2px; }
  .change-wrap { position: relative; }
  .change-wrap.dragging { opacity: 0.4; }
  /* The drop line sits in the 2px gap between rows, so showing it moves nothing. */
  .change-wrap.drop-before::before, .change-wrap.drop-after::after {
    content: ''; position: absolute; left: 6px; right: 6px; height: 2px; border-radius: 1px; background: var(--accent-text); pointer-events: none;
  }
  .change-wrap.drop-before::before { top: -2px; }
  .change-wrap.drop-after::after { bottom: -2px; }
  .quick-switch {
    position: absolute; right: 6px; bottom: 7px; width: 26px; height: 26px;
    background: var(--raised); border: 1px solid var(--line-2); opacity: 0; transition: opacity 0.12s;
  }
  .change-wrap:hover .quick-switch, .quick-switch:focus-visible, .quick-switch.busy { opacity: 1; }
  .row { display: flex; justify-content: space-between; align-items: flex-start; gap: 8px; }
  .row > :global(kbd) { flex-shrink: 0; }
  .id { font-size: 12px; color: var(--accent-text); }
  /* Plain wrapping text, so the dot trails the ID's last word however it wraps. */
  .id-line { flex: 1; min-width: 0; overflow-wrap: anywhere; }
  .live { display: inline-block; width: 6px; height: 6px; margin-left: 6px; border-radius: 3px; background: var(--ok); vertical-align: middle; }
  .title { font-size: 14px; line-height: 1.3; }
  .headline { font-size: 12px; color: var(--muted); }
  .headline.warn { color: var(--warn-text); }
  .headline.ok { color: var(--ok-text); }
  .empty { margin: 0; padding: 0 10px; font-size: 13px; color: var(--muted); line-height: 1.5; }
  .grow-list { flex: 1; min-height: 0; overflow-y: auto; }
  .account {
    display: flex; align-items: center; gap: 4px; margin: 0 -12px -16px; padding: 10px 12px 12px;
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
  .gear { width: 34px; height: 34px; }
  .gear.active { background: var(--selected); color: var(--text); }
</style>
