<script lang="ts">
  import { api, errorText } from '@lib/api'
  import { log } from '@lib/state.svelte'
  import type { ChangeView } from '@lib/types'
  import Icon from './Icon.svelte'

  let { view, onclose }: { view: ChangeView; onclose: () => void } = $props()

  let up = $state('')
  let down = $state('')
  let busy = $state(false)
  let error = $state('')

  const name = (repo: string) => view.legs.find((l) => l.repo === repo)?.name ?? repo
  const inferred = $derived(view.edges.filter((e) => e.via))
  const declared = $derived(view.edges.filter((e) => !e.via))
  const levels = $derived([...new Set(view.legs.map((l) => l.level))].sort((a, b) => a - b))
  const kinds: Record<string, string> = { go: 'go.mod', composer: 'composer.json', npm: 'package.json' }

  async function link() {
    busy = true
    error = ''
    try {
      await api.link(view.id, name(up), name(down))
      log(view.id, `Declared: ${name(up)} merges before ${name(down)}`, 'ok')
      up = down = ''
    } catch (e) {
      error = errorText(e)
    } finally {
      busy = false
    }
  }

  async function unlink(from: string, to: string) {
    busy = true
    error = ''
    try {
      await api.unlink(view.id, name(from), name(to))
      log(view.id, `Removed: ${name(from)} merges before ${name(to)}`, 'muted')
    } catch (e) {
      error = errorText(e)
    } finally {
      busy = false
    }
  }

  function onKey(e: KeyboardEvent) {
    if (e.key === 'Escape' && !busy) onclose()
  }
</script>

<svelte:window onkeydown={onKey} />

<div class="scrim" role="presentation" onclick={() => !busy && onclose()}></div>
<div class="sheet" role="dialog" aria-modal="true" aria-labelledby="edges-title">
  <div class="top">
    <div>
      <div class="mono muted small">{view.id} / merge order</div>
      <h2 id="edges-title">Edit merge order</h2>
    </div>
    <button class="icon-btn" aria-label="Close" disabled={busy} onclick={onclose}><Icon name="close" /></button>
  </div>

  <div class="body">
    <section>
      <div class="eyebrow">Current order</div>
      <div class="order">
        {#each levels as lvl, i}
          {#if i > 0}<span class="muted">→</span>{/if}
          <div class="level">
            {#each view.legs.filter((l) => l.level === lvl) as l (l.repo)}<span class="chip mono">{l.name}</span>{/each}
          </div>
        {/each}
      </div>
    </section>

    <section>
      <div class="eyebrow">From manifests</div>
      {#each inferred as e (e.from + e.to)}
        <div class="edge">
          <span class="mono">{name(e.from)}</span><span class="muted">merges before</span><span class="mono">{name(e.to)}</span>
          <span class="grow"></span>
          <span class="small muted" title="Removing the dependency from {name(e.to)}'s manifest removes this">{kinds[e.kind ?? ''] ?? e.kind}: {e.via}</span>
        </div>
      {:else}
        <p class="muted small">None: no repo in this change requires another's package.</p>
      {/each}
    </section>

    <section>
      <div class="eyebrow">Declared</div>
      {#each declared as e (e.from + e.to)}
        <div class="edge">
          <span class="mono">{name(e.from)}</span><span class="muted">merges before</span><span class="mono">{name(e.to)}</span>
          <span class="grow"></span>
          <button class="btn small" disabled={busy} onclick={() => unlink(e.from, e.to)}>Remove</button>
        </div>
      {:else}
        <p class="muted small">None yet. Declare one when a dependency isn’t in a manifest yet, or isn’t a package at all.</p>
      {/each}
      <div class="add">
        <select class="input" bind:value={up} aria-label="Upstream repo">
          <option value="" disabled>Upstream…</option>
          {#each view.legs as l (l.repo)}<option value={l.repo}>{l.name}</option>{/each}
        </select>
        <span class="muted small">merges before</span>
        <select class="input" bind:value={down} aria-label="Downstream repo">
          <option value="" disabled>Downstream…</option>
          {#each view.legs.filter((l) => l.repo !== up) as l (l.repo)}<option value={l.repo}>{l.name}</option>{/each}
        </select>
        <button class="btn primary small" disabled={!up || !down || up === down || busy} onclick={link}><Icon name="plus" size={14} />Declare</button>
      </div>
      {#if error}<div class="error selectable">{error}</div>{/if}
    </section>
  </div>

  <div class="foot">
    <span class="muted small">Open PRs pick up the new order the next time you publish.</span>
    <button class="btn" onclick={onclose}>Done</button>
  </div>
</div>

<style>
  .scrim { position: fixed; inset: 0; background: var(--scrim); }
  .sheet {
    position: fixed; top: 64px; bottom: 48px; left: 50%; transform: translateX(-50%);
    width: min(760px, calc(100vw - 80px)); background: var(--bg); border: 1px solid var(--line-2);
    border-radius: 16px; display: flex; flex-direction: column; box-shadow: 0 24px 60px var(--shadow);
  }
  .top { display: flex; justify-content: space-between; align-items: flex-start; padding: 22px 24px 0; }
  h2 { margin: 4px 0 0; font-family: var(--display); font-weight: 700; font-size: 24px; }
  .small { font-size: 12px; }
  .body { flex: 1; min-height: 0; overflow: auto; display: flex; flex-direction: column; gap: 20px; padding: 18px 24px; }
  section { display: flex; flex-direction: column; gap: 8px; }
  section p { margin: 0; }
  .order { display: flex; align-items: center; gap: 10px; flex-wrap: wrap; padding: 12px 14px; background: var(--panel); border: 1px solid var(--line); border-radius: 10px; }
  .level { display: flex; flex-direction: column; gap: 4px; }
  .chip { padding: 4px 9px; border: 1px solid var(--line-2); border-radius: 7px; font-size: 12.5px; }
  .edge { display: flex; align-items: center; gap: 10px; min-height: 40px; padding: 0 12px; background: var(--panel); border: 1px solid var(--line); border-radius: 9px; font-size: 13px; }
  .grow { flex: 1; }
  .add { display: flex; align-items: center; gap: 10px; margin-top: 4px; }
  .add select { width: 190px; }
  .error { padding: 8px 12px; border-radius: 8px; background: var(--warn-bg); border: 1px solid var(--warn-border); color: var(--warn-text); font-size: 12.5px; }
  .foot { display: flex; justify-content: space-between; align-items: center; padding: 14px 24px 20px; border-top: 1px solid var(--line); }
</style>
