<script lang="ts">
  import { onMount } from 'svelte'
  import { api } from '@lib/api'
  import { fail } from '@lib/state.svelte'
  import Icon from './Icon.svelte'

  let { id, onclose, onsync }: { id: string; onclose: () => void; onsync: () => void } = $props()

  let plan = $state<{ leg: string; commands: string[]; skip: string }[] | null>(null)
  const rebasing = $derived((plan ?? []).filter((p) => !p.skip).length)

  onMount(() => {
    api.syncPlan(id).then((p) => (plan = p)).catch((e) => { fail(e); onclose() })
  })

  function onKey(e: KeyboardEvent) {
    if (e.key === 'Escape') onclose()
  }
</script>

<svelte:window onkeydown={onKey} />

<div class="scrim" role="presentation" onclick={onclose}></div>
<div class="sheet" role="dialog" aria-modal="true" aria-labelledby="sync-title">
  <div class="top">
    <div>
      <div class="mono muted small">{id} / sync</div>
      <h2 id="sync-title">Sync with each base branch</h2>
    </div>
    <button class="icon-btn" aria-label="Close" onclick={onclose}><Icon name="close" /></button>
  </div>
  <p class="muted lead">Rebases each clean leg onto its base so it has the latest changes. A conflict is aborted and leaves the leg as it was, and you can undo the sync until you next publish.</p>
  {#if !plan}
    <p class="muted small fetching"><Icon name="running" size={14} spin />Fetching every leg to see what's new…</p>
  {:else}
    <ul class="legs">
      {#each plan as p (p.leg)}
        <li>
          <div class="leg-head">
            <span class="mono strong">{p.leg}</span>
            {#if p.skip}<span class="small muted">{p.skip}</span>{:else}<span class="small ok">will rebase</span>{/if}
          </div>
          {#if !p.skip}
            <pre class="mono cmds selectable">{p.commands.map((c) => `$ ${c}`).join('\n')}</pre>
          {/if}
        </li>
      {/each}
    </ul>
  {/if}
  <div class="foot">
    <button class="btn" onclick={onclose}>Cancel</button>
    <button class="btn primary" disabled={!plan || rebasing === 0} onclick={() => { onclose(); onsync() }}>
      <Icon name="sync" />{!plan ? 'Sync' : rebasing === 0 ? 'Nothing to rebase' : `Rebase ${rebasing} leg${rebasing === 1 ? '' : 's'}`}
    </button>
  </div>
</div>

<style>
  .scrim { position: fixed; inset: 0; background: var(--scrim); z-index: 30; }
  .sheet {
    position: fixed; top: 12vh; left: 50%; transform: translateX(-50%); z-index: 31; width: min(620px, calc(100vw - 48px));
    max-height: 76vh; overflow: auto; display: flex; flex-direction: column; gap: 16px; padding: 22px 24px;
    background: var(--bg); border: 1px solid var(--line-2); border-radius: 16px; box-shadow: 0 24px 60px var(--shadow);
  }
  .top { display: flex; justify-content: space-between; align-items: flex-start; }
  h2 { margin: 4px 0 0; font-family: var(--display); font-weight: 700; font-size: 24px; }
  .small { font-size: 12.5px; }
  .lead { margin: 0; font-size: 13.5px; line-height: 1.55; }
  .fetching { display: flex; align-items: center; gap: 8px; margin: 0; }
  .legs { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 8px; }
  .legs li { padding: 10px 12px; border: 1px solid var(--line); border-radius: 10px; background: var(--panel); display: flex; flex-direction: column; gap: 8px; }
  .leg-head { display: flex; justify-content: space-between; gap: 12px; align-items: baseline; }
  .strong { font-weight: 500; }
  .cmds { margin: 0; padding: 8px 10px; border-radius: 8px; background: var(--nav); font-size: 12px; color: var(--text-2); white-space: pre-wrap; }
  .foot { display: flex; justify-content: flex-end; gap: 10px; }
</style>
