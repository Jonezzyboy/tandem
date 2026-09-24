<script lang="ts">
  import { api } from '@lib/api'
  import { fail, loadCleanPlan } from '@lib/state.svelte'
  import type { CleanItem, CleanResult } from '@lib/types'
  import Icon from './Icon.svelte'

  let { items, onclose }: { items: CleanItem[]; onclose: () => void } = $props()

  let busy = $state(false)
  let results = $state<CleanResult[] | null>(null)
  const count = $derived(items.reduce((n, it) => n + it.worktrees.length + it.branches.length + it.files.length, 0))

  async function clean() {
    busy = true
    try {
      results = await api.clean(items.map((it) => it.id))
      loadCleanPlan()
    } catch (e) {
      fail(e)
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
<div class="sheet" role="dialog" aria-modal="true" aria-labelledby="clean-title">
  <div class="top">
    <div>
      <div class="mono muted small">housekeeping</div>
      <h2 id="clean-title">{results ? 'Cleaned up' : `Delete ${count} items from ${items.length} landed change${items.length === 1 ? '' : 's'}`}</h2>
    </div>
    <button class="icon-btn" aria-label="Close" disabled={busy} onclick={onclose}><Icon name="close" /></button>
  </div>

  <div class="body">
    {#if results}
      {#each results as r (r.id)}
        <div class="result">
          {#if r.ok}<Icon name="check" color="var(--ok)" />{:else}<Icon name="x" color="var(--warn)" />{/if}
          <span class="mono">{r.id}</span>
          <span class="small selectable" class:warn={!r.ok} class:muted={r.ok}>{r.message}</span>
        </div>
      {/each}
    {:else}
      <p class="muted small">Every path below is removed exactly as listed. A worktree git reports as modified is refused, and the change stays.</p>
      {#each items as it (it.id)}
        <section>
          <div class="head"><span class="mono accent">{it.id}</span><span>{it.title}</span></div>
          <ul class="mono selectable">
            {#each it.worktrees as w}<li><span class="verb">worktree</span>{w}</li>{/each}
            {#each it.branches as b}<li><span class="verb">branch</span>{b}</li>{/each}
            {#each it.files as f}<li><span class="verb">file</span>{f}</li>{/each}
            <li><span class="verb">dir</span><span>{it.dir} <span class="muted">(only if empty)</span></span></li>
            {#each it.kept as k}<li class="muted"><span class="verb">keep</span>{k} (PR closed unmerged)</li>{/each}
          </ul>
        </section>
      {/each}
    {/if}
  </div>

  <div class="foot">
    {#if results}
      <span></span><button class="btn" onclick={onclose}>Done</button>
    {:else}
      <button class="btn" disabled={busy} onclick={onclose}>Cancel</button>
      <button class="btn danger" disabled={busy} onclick={clean}>
        <Icon name="close" spin={busy} />Delete {count} items
      </button>
    {/if}
  </div>
</div>

<style>
  .scrim { position: fixed; inset: 0; background: var(--scrim); }
  .sheet {
    position: fixed; top: 80px; bottom: 64px; left: 50%; transform: translateX(-50%);
    width: min(860px, calc(100vw - 80px)); background: var(--bg); border: 1px solid var(--line-2);
    border-radius: 16px; display: flex; flex-direction: column; box-shadow: 0 24px 60px var(--shadow);
  }
  .top { display: flex; justify-content: space-between; align-items: flex-start; padding: 22px 24px 0; }
  h2 { margin: 4px 0 0; font-family: var(--display); font-weight: 700; font-size: 24px; }
  .small { font-size: 12px; }
  .body { flex: 1; min-height: 0; overflow: auto; display: flex; flex-direction: column; gap: 16px; padding: 18px 24px; }
  .body p { margin: 0; }
  section { display: flex; flex-direction: column; gap: 8px; padding: 14px 16px; background: var(--panel); border: 1px solid var(--line); border-radius: 12px; }
  .head { display: flex; gap: 12px; align-items: baseline; }
  .accent { color: var(--accent-text); }
  ul { margin: 0; padding: 0; list-style: none; display: flex; flex-direction: column; gap: 4px; font-size: 12px; color: var(--text-2); }
  li { display: grid; grid-template-columns: 72px minmax(0, 1fr); gap: 8px; word-break: break-all; }
  .verb { color: var(--muted); }
  .result { display: flex; gap: 10px; align-items: center; padding: 10px 14px; background: var(--panel); border: 1px solid var(--line); border-radius: 10px; font-size: 13px; }
  .foot { display: flex; justify-content: space-between; align-items: center; padding: 14px 24px 20px; border-top: 1px solid var(--line); }
  .danger { background: var(--danger); border-color: var(--danger); color: var(--on-accent); }
  .danger:hover:not(:disabled) { background: var(--danger-hover); }
</style>
