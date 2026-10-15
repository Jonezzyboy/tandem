<script lang="ts">
  import { tick } from 'svelte'
  import { api } from '@lib/api'
  import { app, currentId, navigate } from '@lib/state.svelte'
  import { actions, keycaps, shortcutFor } from '@lib/settings.svelte'
  import Icon from './Icon.svelte'
  import Kbd from './Kbd.svelte'

  let { run, onclose }: { run: (action: string) => void; onclose: () => void } = $props()

  interface Item {
    key: string
    kind: string
    label: string
    detail?: string
    keys?: string[]
    go: () => void
  }

  let query = $state('')
  let selected = $state(0)
  let listEl = $state<HTMLElement>()

  const items = $derived.by<Item[]>(() => {
    const out: Item[] = []
    for (const c of app.changes) {
      out.push({ key: `change:${c.id}`, kind: 'Change', label: `${c.id} · ${c.title || 'Untitled change'}`, detail: c.headline,
        go: () => navigate({ name: 'change', id: c.id }) })
    }
    for (const a of actions) {
      if (a.id === 'palette' || (a.scope === 'change' && !currentId())) continue
      out.push({ key: `action:${a.id}`, kind: 'Action', label: a.label, keys: keycaps(shortcutFor(a.id)), go: () => run(a.id) })
    }
    out.push({ key: 'action:setup', kind: 'Action', label: 'Open the setup checklist', go: () => navigate({ name: 'setup' }) })
    for (const p of app.inbox?.review ?? []) {
      out.push({ key: `review:${p.url}`, kind: 'Review', label: `${p.repo}#${p.number} · ${p.title}`, detail: `by ${p.author}`,
        go: () => (p.changeId ? navigate({ name: 'change', id: p.changeId }) : api.openURL(p.url)) })
    }
    for (const p of app.inbox?.mine ?? []) {
      out.push({ key: `mine:${p.url}`, kind: 'Your PR', label: `${p.repo}#${p.number} · ${p.title}`,
        go: () => (p.changeId ? navigate({ name: 'change', id: p.changeId }) : api.openURL(p.url)) })
    }
    return out
  })

  const matches = $derived.by(() => {
    const terms = query.toLowerCase().split(/\s+/).filter(Boolean)
    return items.filter((it) => terms.every((t) => `${it.kind} ${it.label} ${it.detail ?? ''}`.toLowerCase().includes(t))).slice(0, 60)
  })

  $effect(() => {
    query
    selected = 0
  })

  function choose(it: Item | undefined) {
    if (!it) return
    onclose()
    it.go()
  }

  async function onKey(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      e.preventDefault()
      onclose()
    } else if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
      e.preventDefault()
      const n = matches.length
      if (!n) return
      selected = (selected + (e.key === 'ArrowDown' ? 1 : n - 1)) % n
      await tick()
      listEl?.querySelector('[aria-selected="true"]')?.scrollIntoView({ block: 'nearest' })
    } else if (e.key === 'Enter') {
      e.preventDefault()
      choose(matches[selected])
    }
  }

  function focus(el: HTMLInputElement) {
    el.focus()
  }
</script>

<div class="scrim" role="presentation" onclick={onclose}></div>
<div class="palette" role="dialog" aria-modal="true" aria-label="Command palette">
  <div class="search-row">
    <Icon name="search" color="var(--muted)" />
    <input bind:value={query} use:focus onkeydown={onKey} placeholder="Jump to a change, PR or action…" aria-label="Search"
      role="combobox" aria-expanded="true" aria-controls="palette-list" spellcheck="false" />
  </div>
  <div class="list" id="palette-list" role="listbox" bind:this={listEl}>
    {#each matches as it, i (it.key)}
      <button class="item" role="option" aria-selected={i === selected} onmousemove={() => (selected = i)} onclick={() => choose(it)}>
        <span class="kind">{it.kind}</span>
        <span class="label">{it.label}{#if it.detail}<span class="muted">{` · ${it.detail}`}</span>{/if}</span>
        {#if it.keys?.length}<Kbd keys={it.keys} />{/if}
      </button>
    {:else}
      <p class="muted none">Nothing matches.</p>
    {/each}
  </div>
  <div class="foot muted"><span>↑↓ to move</span><span>↩ to open</span><span>Esc to close</span></div>
</div>

<style>
  .scrim { position: fixed; inset: 0; background: var(--scrim); z-index: 40; }
  .palette {
    position: fixed; top: 14vh; left: 50%; transform: translateX(-50%); z-index: 41;
    width: min(640px, calc(100vw - 48px)); max-height: 64vh; display: flex; flex-direction: column;
    background: var(--bg); border: 1px solid var(--line-2); border-radius: 14px; box-shadow: 0 24px 60px var(--shadow); overflow: hidden;
  }
  .search-row { display: flex; align-items: center; gap: 10px; padding: 14px 16px; border-bottom: 1px solid var(--line); }
  .search-row input { flex: 1; border: 0; background: none; outline: none; font-size: 15px; color: var(--text); }
  .list { flex: 1; min-height: 0; overflow-y: auto; padding: 6px; }
  .item {
    display: flex; align-items: center; gap: 12px; width: 100%; padding: 8px 10px; border: 0; border-radius: 8px;
    background: none; color: var(--text); text-align: left; font-size: 13.5px; cursor: pointer;
  }
  .item[aria-selected='true'] { background: var(--selected); }
  .kind { width: 64px; flex-shrink: 0; font-size: 11px; color: var(--muted); text-transform: uppercase; letter-spacing: 0.04em; }
  .label { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .none { padding: 10px; margin: 0; font-size: 13px; }
  .foot { display: flex; gap: 16px; padding: 8px 16px; border-top: 1px solid var(--line); font-size: 11.5px; }
</style>
