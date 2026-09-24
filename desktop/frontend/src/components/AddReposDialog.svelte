<script lang="ts">
  import { api } from '@lib/api'
  import { fail, log } from '@lib/state.svelte'
  import type { ChangeView, StartItem } from '@lib/types'
  import Icon from './Icon.svelte'
  import RepoPicker from './RepoPicker.svelte'

  let { view, onclose, onpublish }: { view: ChangeView; onclose: () => void; onpublish: () => void } = $props()

  let selected = $state<string[]>([])
  let adding = $state(false)
  let results = $state<StartItem[] | null>(null)
  const added = $derived((results ?? []).filter((r) => r.ok && !r.existing).length)

  async function add() {
    adding = true
    try {
      results = await api.addRepos(view.id, selected)
      for (const r of results) log(view.id, `Added ${r.repo}: ${r.message}`, r.ok ? 'ok' : 'warn')
    } catch (e) {
      fail(e)
    } finally {
      adding = false
    }
  }

  function onKey(e: KeyboardEvent) {
    if (e.key === 'Escape' && !adding) onclose()
  }
</script>

<svelte:window onkeydown={onKey} />

<div class="scrim" role="presentation" onclick={() => !adding && onclose()}></div>
<div class="sheet" role="dialog" aria-modal="true" aria-labelledby="add-title">
  <div class="top">
    <div>
      <div class="mono muted small">{view.id} / add repos</div>
      <h2 id="add-title">{results ? 'Added to the change' : 'Add repos to this change'}</h2>
    </div>
    <button class="icon-btn" aria-label="Close" disabled={adding} onclick={onclose}><Icon name="close" /></button>
  </div>

  {#if results}
    <div class="body results">
      {#each results as r (r.repo)}
        <div class="result">
          {#if r.ok}<Icon name="check" color="var(--ok)" />{:else}<Icon name="x" color="var(--warn)" />{/if}
          <span class="mono">{r.repo}</span>
          <span class="small selectable" class:warn={!r.ok} class:muted={r.ok}>{r.message}</span>
        </div>
      {/each}
      {#if added}
        <p class="note">The merge order now includes {added === 1 ? 'it' : 'them'}. Publish to open {added === 1 ? 'its PR' : 'their PRs'} and rewrite the merge-order section in the PRs already open.</p>
      {/if}
    </div>
    <div class="foot">
      <button class="btn" onclick={onclose}>Done</button>
      {#if added}<button class="btn primary" onclick={onpublish}><Icon name="send" />Publish PRs</button>{/if}
    </div>
  {:else}
    <div class="body">
      <p class="muted small">Each repo gets the <span class="mono">{view.branch}</span> branch from a freshly fetched base and has it checked out, unless it holds uncommitted work. Tandem works out where it goes in the merge order from its manifests.</p>
      <RepoPicker bind:selected exclude={view.legs.map((l) => l.repo)} />
    </div>
    <div class="foot">
      <span class="muted small">{selected.length ? selected.join(', ') : 'Nothing picked yet'}</span>
      <button class="btn primary" disabled={selected.length === 0 || adding} onclick={add}>
        <Icon name="plus" spin={adding} />Add {selected.length || ''} repo{selected.length === 1 ? '' : 's'}
      </button>
    </div>
  {/if}
</div>

<style>
  .scrim { position: fixed; inset: 0; background: var(--scrim); }
  .sheet {
    position: fixed; top: 56px; bottom: 40px; left: 50%; transform: translateX(-50%);
    width: min(760px, calc(100vw - 80px)); background: var(--bg); border: 1px solid var(--line-2);
    border-radius: 16px; display: flex; flex-direction: column; box-shadow: 0 24px 60px var(--shadow);
  }
  .top { display: flex; justify-content: space-between; align-items: flex-start; padding: 22px 24px 0; }
  h2 { margin: 4px 0 0; font-family: var(--display); font-weight: 700; font-size: 24px; }
  .small { font-size: 12px; }
  .body { flex: 1; min-height: 0; display: flex; flex-direction: column; gap: 14px; padding: 16px 24px; }
  .body p { margin: 0; line-height: 1.5; }
  .results { gap: 10px; overflow: auto; }
  .result { display: grid; grid-template-columns: 16px auto; gap: 4px 10px; align-items: center; font-size: 13px; padding: 10px 12px; background: var(--panel); border: 1px solid var(--line); border-radius: 10px; }
  .result span:last-child { grid-column: 2; }
  .note { font-size: 13px; color: var(--text-2); }
  .foot { display: flex; justify-content: space-between; align-items: center; gap: 16px; padding: 14px 24px 20px; border-top: 1px solid var(--line); }
  .foot .muted { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
</style>
