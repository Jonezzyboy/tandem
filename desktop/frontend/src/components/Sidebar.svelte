<script lang="ts">
  import { tick } from 'svelte'
  import { api } from '@lib/api'
  import { app, checkOut, fail, navigate, switching } from '@lib/state.svelte'
  import Icon from './Icon.svelte'
  import Kbd from './Kbd.svelte'
  import Avatar from './Avatar.svelte'
  import { keycaps, prefs, shortcutFor } from '@lib/settings.svelte'
  import type { ChangeSummary } from '@lib/types'

  const reviewCount = $derived(app.inbox?.review?.length ?? 0)

  // Pointer-driven rather than HTML5 drag-and-drop. A grabbed row lifts off as a floating card that
  // follows the pointer anywhere; its place in the list becomes an invisible placeholder that the
  // other rows slide around. Over the list it previews where it would land; let go off the list and it
  // goes back where it was.
  const ROW_GAP = 2
  const THRESHOLD = 4
  // How far outside the list, sideways, the card still counts as over it.
  const SLOP = 24
  interface Drag {
    id: string; from: number; to: number; tops: number[]; heights: number[]; el: HTMLElement
    startX: number; startY: number; x: number; y: number; grabX: number; grabY: number; width: number; moved: boolean
  }
  let drag = $state<Drag | null>(null)
  // The card gliding into its slot after a drop; its row stays hidden until it lands.
  let landing = $state<{ id: string; left: number; top: number } | null>(null)
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

  // The offset that puts row i where it sits while dragging: the dragged row's placeholder at the
  // slot it would land in, and the rows between there and its old place shifted to make room.
  function shift(i: number): string | null {
    if (!drag?.moved) return null
    const { from, to, tops, heights } = drag
    const room = heights[from] + ROW_GAP
    if (i === from) {
      const top = to > from ? tops[to] + heights[to] - heights[from] : tops[to]
      return `translateY(${top - tops[from]}px)`
    }
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
    if (e.button !== 0 || landing || (e.target as HTMLElement).closest('.quick-switch') || !listEl) return
    const el = e.currentTarget as HTMLElement
    const r = el.getBoundingClientRect()
    const rects = [...listEl.querySelectorAll<HTMLElement>(':scope > .change-wrap')].map((w) => w.getBoundingClientRect())
    drag = {
      id, from: i, to: i, tops: rects.map((q) => q.top), heights: rects.map((q) => q.height), el,
      startX: e.clientX, startY: e.clientY, x: e.clientX, y: e.clientY, grabX: e.clientX - r.left, grabY: e.clientY - r.top, width: r.width, moved: false,
    }
    window.addEventListener('pointermove', onPointerMove)
    window.addEventListener('pointerup', onPointerUp)
    window.addEventListener('pointercancel', onPointerUp)
  }

  // Where the card is drawn: under the pointer, but never past the window's edge.
  function cardAt(d: Drag): { left: number; top: number } {
    return {
      left: Math.min(Math.max(d.x - d.grabX, 0), Math.max(window.innerWidth - d.width, 0)),
      top: Math.min(Math.max(d.y - d.grabY, 0), Math.max(window.innerHeight - d.heights[d.from], 0)),
    }
  }

  function overList(d: Drag): boolean {
    const r = listEl!.getBoundingClientRect()
    const last = d.tops.length - 1
    return d.x > r.left - SLOP && d.x < r.right + SLOP && d.y > d.tops[0] - 40 && d.y < d.tops[last] + d.heights[last] + 40
  }

  function onPointerMove(e: PointerEvent) {
    if (!drag) return
    if (!drag.moved && Math.hypot(e.clientX - drag.startX, e.clientY - drag.startY) < THRESHOLD) return
    if (!drag.moved) {
      // Keeps moves and the release coming while the pointer is outside the window.
      try { drag.el.setPointerCapture(e.pointerId) } catch {}
    }
    drag.moved = true
    drag.x = e.clientX
    drag.y = e.clientY
    const { from, tops, heights } = drag
    if (!overList(drag)) {
      drag.to = from
      return
    }
    // A row makes way once the card's leading edge is a third of the way into it, so rows slide aside
    // before the card covers them.
    const top = drag.y - drag.grabY
    const bottom = top + heights[from]
    const below = tops.filter((t, j) => j > from && bottom > t + heights[j] / 3).length
    const above = tops.filter((t, j) => j < from && top < t + heights[j] * (2 / 3)).length
    drag.to = from + below - above
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
    const to = e.type === 'pointerup' && overList(d) ? d.to : d.from
    // Rows drop their offsets as the list reorders under them; animating that would double the move.
    const rows = [...listEl!.querySelectorAll<HTMLElement>(':scope > .change-wrap')]
    for (const r of rows) r.style.transition = 'none'
    landing = { id: d.id, ...cardAt(d) }
    drag = null
    if (to !== d.from) move(d.id, to > d.from ? to + 1 : to)
    await tick()
    rows.forEach((r) => r.getBoundingClientRect())
    for (const r of rows) r.style.transition = ''
    // Glide the card into the row's slot, then show the row in its place.
    const slotRect = d.el.getBoundingClientRect()
    requestAnimationFrame(() => {
      if (landing) landing = { id: d.id, left: slotRect.left, top: slotRect.top }
    })
    setTimeout(() => (landing = null), 200)
  }

  async function onKey(e: KeyboardEvent, id: string, i: number) {
    if (!e.altKey || (e.key !== 'ArrowUp' && e.key !== 'ArrowDown')) return
    e.preventDefault()
    await move(id, e.key === 'ArrowUp' ? i - 1 : i + 2)
    await tick()
    document.querySelector<HTMLElement>(`[data-change="${CSS.escape(id)}"]`)?.focus()
  }
</script>

{#snippet rowBody(c: ChangeSummary, n: number)}
  <span class="row">
    <span class="id-line">
      <span class="mono id">{c.id}</span>
      {#if c.checkedOut}<span class="live" title="Checked out in every repo" aria-label="checked out"></span>{/if}
    </span>
    {#if n < 9}<Kbd keys={['⌘', String(n + 1)]} />{/if}
  </span>
  <span class="title">{c.title || 'Untitled change'}</span>
  <span class="headline {c.tone}">{c.headline}</span>
{/snippet}

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
      <div class="change-wrap" role="listitem" class:placeholder={(drag?.moved && drag.id === c.id) || landing?.id === c.id}
        style:transform={shift(i)} onpointerdown={(e) => onPointerDown(e, c.id, i)}>
        <button
          class="change"
          data-change={c.id}
          class:active={app.route.name === 'change' && app.route.id === c.id}
          onclick={() => { if (!justDragged) navigate({ name: 'change', id: c.id }) }}
          onkeydown={(e) => onKey(e, c.id, i)}
          title="Drag, or ⌥↑ ⌥↓, to reorder"
        >
          {@render rowBody(c, slot(i))}
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
  {#if (drag?.moved || landing) && app.changes.find((c) => c.id === (drag?.id ?? landing?.id))}
    {@const c = app.changes.find((x) => x.id === (drag?.id ?? landing?.id))!}
    {@const at = drag?.moved ? cardAt(drag) : landing!}
    <div class="card" class:landing={!drag?.moved} aria-hidden="true"
      style:width="{drag?.width ?? listEl?.querySelector('.change-wrap')?.getBoundingClientRect().width}px"
      style:transform="translate({at.left}px, {at.top}px)">
      <div class="change">{@render rowBody(c, drag?.moved ? drag.to : app.changes.indexOf(c))}</div>
    </div>
  {/if}
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
  /* WebKit leaves rows unpainted mid-slide inside the scrolling list unless each has its own layer. */
  .list.dragging .change-wrap { will-change: transform; }
  .change-wrap.placeholder { visibility: hidden; }
  :global(body:has(.card)) { cursor: grabbing; }
  .card {
    position: fixed; top: 0; left: 0; z-index: 50; pointer-events: none; border-radius: 8px; background: var(--raised);
    scale: 1.03; box-shadow: 0 0 0 1px var(--line-2), 0 14px 32px var(--shadow);
    transition: scale 0.15s ease-out, box-shadow 0.15s ease-out;
  }
  @starting-style { .card { scale: 1; box-shadow: 0 0 0 1px var(--line-2); } }
  .card .change { cursor: grabbing; background: transparent; }
  .card.landing {
    scale: 1; box-shadow: 0 0 0 1px var(--line-2);
    transition: transform 0.18s cubic-bezier(0.2, 0.8, 0.2, 1), scale 0.18s ease-out, box-shadow 0.18s ease-out;
  }
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
