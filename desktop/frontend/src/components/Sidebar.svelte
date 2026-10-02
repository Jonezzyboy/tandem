<script lang="ts">
  import { tick } from 'svelte'
  import { api } from '@lib/api'
  import { app, checkOut, fail, navigate, switching } from '@lib/state.svelte'
  import Icon from './Icon.svelte'
  import Kbd from './Kbd.svelte'
  import Avatar from './Avatar.svelte'
  import { keycaps, prefs, shortcutFor } from '@lib/settings.svelte'

  const reviewCount = $derived(app.inbox?.review?.length ?? 0)

  // Pointer-driven rather than HTML5 drag-and-drop: the row follows the pointer, the others slide aside
  // live, and releasing anywhere commits, so there are no dead gaps between drop targets.
  const ROW_GAP = 2
  const THRESHOLD = 4
  interface Drag { id: string; from: number; to: number; startY: number; dy: number; tops: number[]; heights: number[]; moved: boolean; el: HTMLElement }
  let drag = $state<Drag | null>(null)
  let listEl = $state<HTMLElement>()
  let justDragged = false

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

  function shift(i: number): string | null {
    if (!drag?.moved) return null
    const { from, to, dy, heights } = drag
    if (i === from) return `translateY(${dy}px)`
    const room = heights[from] + ROW_GAP
    if (from < to && i > from && i <= to) return `translateY(${-room}px)`
    if (to < from && i >= to && i < from) return `translateY(${room}px)`
    return null
  }

  // Where row i sits while dragging, so its ⌘ shortcut previews the order it would land in.
  function slot(i: number): number {
    if (!drag?.moved) return i
    const { from, to } = drag
    if (i === from) return to
    if (from < to && i > from && i <= to) return i - 1
    if (to < from && i >= to && i < from) return i + 1
    return i
  }

  function onPointerDown(e: PointerEvent, id: string, i: number) {
    if (e.button !== 0 || (e.target as HTMLElement).closest('.quick-switch') || !listEl) return
    const rects = [...listEl.querySelectorAll<HTMLElement>(':scope > .change-wrap')].map((w) => w.getBoundingClientRect())
    drag = { id, from: i, to: i, startY: e.clientY, dy: 0, tops: rects.map((r) => r.top), heights: rects.map((r) => r.height), moved: false, el: e.currentTarget as HTMLElement }
    window.addEventListener('pointermove', onPointerMove)
    window.addEventListener('pointerup', onPointerUp)
    window.addEventListener('pointercancel', onPointerUp)
  }

  function onPointerMove(e: PointerEvent) {
    if (!drag) return
    const { from, tops, heights } = drag
    const dy = e.clientY - drag.startY
    if (!drag.moved && Math.abs(dy) < THRESHOLD) return
    drag.moved = true
    const last = tops.length - 1
    const top = Math.min(Math.max(tops[from] + dy, tops[0]), tops[last] + heights[last] - heights[from])
    drag.dy = top - tops[from]
    const centre = top + heights[from] / 2
    drag.to = tops.filter((t, j) => j !== from && t + heights[j] / 2 < centre).length
  }

  async function onPointerUp(e: PointerEvent) {
    window.removeEventListener('pointermove', onPointerMove)
    window.removeEventListener('pointerup', onPointerUp)
    window.removeEventListener('pointercancel', onPointerUp)
    const d = drag
    if (!d?.moved) {
      drag = null
      return
    }
    // The click that follows a drag must not open the change.
    justDragged = true
    setTimeout(() => (justDragged = false))
    // Rows drop their slide offsets as the list reorders under them; animating that would double the move.
    const rows = [...listEl!.querySelectorAll<HTMLElement>(':scope > .change-wrap')]
    for (const r of rows) r.style.transition = 'none'
    const before = d.el.getBoundingClientRect().top
    drag = null
    if (e.type === 'pointerup' && d.to !== d.from) move(d.id, d.to > d.from ? d.to + 1 : d.to)
    await tick()
    // Then glide the dropped row from where it was let go into its slot.
    d.el.style.transform = `translateY(${before - d.el.getBoundingClientRect().top}px)`
    d.el.getBoundingClientRect()
    requestAnimationFrame(() => {
      for (const r of rows) r.style.transition = ''
      d.el.style.transform = ''
    })
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
    <div class="list" role="list" aria-labelledby="my-changes" bind:this={listEl} class:dragging={drag?.moved}>
    {#each app.changes as c, i (c.id)}
      <div class="change-wrap" role="listitem" class:lifted={drag?.moved && drag.id === c.id} class:sliding={drag?.moved && drag.id !== c.id}
        style:transform={shift(i)} onpointerdown={(e) => onPointerDown(e, c.id, i)}>
        <button
          class="change"
          data-change={c.id}
          class:active={app.route.name === 'change' && app.route.id === c.id}
          onclick={() => { if (!justDragged) navigate({ name: 'change', id: c.id }) }}
          onkeydown={(e) => onKey(e, c.id, i)}
          title="Drag, or ⌥↑ ⌥↓, to reorder"
        >
          <span class="row">
            <span class="id-line">
              <span class="mono id">{c.id}</span>
              {#if c.checkedOut}<span class="live" title="Checked out in every repo" aria-label="checked out"></span>{/if}
            </span>
            {#if slot(i) < 9}<Kbd keys={['⌘', String(slot(i) + 1)]} />{/if}
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
  .change-wrap { transition: transform 0.18s cubic-bezier(0.2, 0.8, 0.2, 1); touch-action: none; }
  .list.dragging, .list.dragging .change { cursor: grabbing; }
  .change-wrap.lifted {
    z-index: 2; transition: none; border-radius: 8px; background: var(--raised);
    box-shadow: 0 0 0 1px var(--line-2), 0 10px 24px var(--shadow);
  }
  .change-wrap.lifted .change { background: transparent; }
  /* The lift itself: on the inner button, since the row's own transform tracks the pointer. */
  .change-wrap .change { transition: transform 0.15s ease-out; }
  .change-wrap.lifted .change { transform: scale(1.025); }
  .change-wrap.lifted { transition: box-shadow 0.15s ease-out, background 0.15s ease-out; }
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
  /* Bleeds into nav's padding so a lifted row's scale and shadow have room without a sideways scrollbar. */
  .grow-list { flex: 1; min-height: 0; overflow-y: auto; overflow-x: hidden; margin: 0 -12px; padding: 0 12px 12px; }
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
