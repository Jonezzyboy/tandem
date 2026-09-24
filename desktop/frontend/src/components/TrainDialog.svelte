<script lang="ts">
  import { onMount } from 'svelte'
  import { api, errorText } from '@lib/api'
  import { app, fail } from '@lib/state.svelte'
  import type { ChangeView, TrainPlan } from '@lib/types'
  import Icon from './Icon.svelte'

  let { view, onclose }: { view: ChangeView; onclose: () => void } = $props()

  let plan = $state<TrainPlan | null>(null)
  let loading = $state(false)
  let method = $state<'squash' | 'merge' | 'rebase'>('squash')

  const run = $derived(app.trains[view.id])
  const running = $derived(run?.running ?? false)
  const levels = $derived([...new Set((plan?.legs ?? []).map((l) => l.level))].sort((a, b) => a - b))
  const repins = $derived(view.edges.some((e) => e.kind === 'go'))

  async function load() {
    loading = true
    try {
      plan = await api.trainPlan(view.id)
    } catch (e) {
      fail(e)
    } finally {
      loading = false
    }
  }

  async function start() {
    app.trains[view.id] = { running: true, events: [], error: '' }
    try {
      await api.startTrain(view.id, method)
    } catch (e) {
      app.trains[view.id].error = errorText(e)
    } finally {
      app.trains[view.id].running = false
      load()
    }
  }

  function onKey(e: KeyboardEvent) {
    if (e.key === 'Escape' && !running) onclose()
  }

  onMount(() => {
    if (!running) load()
  })
</script>

<svelte:window onkeydown={onKey} />

<div class="scrim" role="presentation" onclick={() => !running && onclose()}></div>
<div class="sheet" role="dialog" aria-modal="true" aria-labelledby="train-title">
  <div class="top">
    <div>
      <div class="mono muted small">{view.id} / merge train</div>
      <h2 id="train-title">Merge in dependency order</h2>
    </div>
    <button class="icon-btn" aria-label="Close" disabled={running} onclick={onclose}><Icon name="close" /></button>
  </div>

  <div class="body">
    <div class="plan">
      <div class="eyebrow">Plan</div>
      {#if !plan}
        <p class="muted small">Reading each PR…</p>
      {:else}
        {#each levels as lvl}
          <div class="level">
            <div class="badge mono">{lvl}</div>
            <div class="legs">
              {#each plan.legs.filter((l) => l.level === lvl) as l (l.repo)}
                <div class="leg">
                  <span class="mono">{l.name}</span>
                  {#if l.pr}
                    <a href={l.url} class="mono small" onclick={(e) => { e.preventDefault(); api.openURL(l.url) }}>#{l.pr}</a>
                  {/if}
                  <span class="grow"></span>
                  {#if l.problems.length}
                    <span class="small warn">{l.problems.join(' · ')}</span>
                  {:else if l.merged}
                    <span class="small muted">already merged</span>
                  {:else}
                    <span class="small ok">ready</span>
                  {/if}
                </div>
              {/each}
            </div>
          </div>
        {/each}
        {#if repins}
          <p class="muted small note">After each level merges, the Go legs that depend on it are re-pinned to its merge commit and pushed, and they merge only once CI passes on that push.</p>
        {/if}
      {/if}
    </div>

    <div class="log">
      <div class="eyebrow">Progress</div>
      {#if !run || run.events.length === 0}
        <p class="muted small">{running ? 'Starting…' : 'Nothing has run yet. Stopping part-way is safe: merged legs stay merged, and the next run carries on.'}</p>
      {:else}
        {#each run.events as ev, i (i)}
          {@const current = running && i === run.events.length - 1}
          <div class="event" class:past={!current && (ev.phase === 'waiting' || ev.phase === 'merging')}>
            {#if ev.phase === 'done' || ev.phase === 'merged' || ev.phase === 'pinned'}<Icon name="check" color="var(--ok)" />
            {:else if current}<Icon name="running" spin />
            {:else}<Icon name="clock" color="var(--muted)" />{/if}
            <span class="mono name">{ev.leg || 'train'}</span>
            <span class="small">{ev.phase === 'done' ? ev.detail : `${ev.phase} ${ev.detail}`}</span>
          </div>
        {/each}
      {/if}
      {#if run?.error}<pre class="error selectable">{run.error}</pre>{/if}
    </div>
  </div>

  <div class="foot">
    <fieldset class="methods" disabled={running}>
      <legend class="small muted">Merge with</legend>
      {#each ['squash', 'merge', 'rebase'] as m}
        <label class="method"><input type="radio" name="method" value={m} bind:group={method} />{m}</label>
      {/each}
    </fieldset>
    {#if running}
      <button class="btn" onclick={() => api.cancelTrain(view.id)}><Icon name="close" />Stop after the current step</button>
    {:else}
      <button class="btn primary" disabled={loading || !plan || plan.blocked > 0 || plan.toMerge === 0} onclick={start}>
        <Icon name="send" />
        {#if !plan}Checking…{:else if plan.blocked > 0}{plan.blocked} blocked{:else if plan.toMerge === 0}All merged{:else}Merge {plan.toMerge} PR{plan.toMerge === 1 ? '' : 's'}{/if}
      </button>
    {/if}
  </div>
</div>

<style>
  .scrim { position: fixed; inset: 0; background: rgba(5, 6, 8, 0.6); }
  .sheet {
    position: fixed; top: 64px; bottom: 48px; left: 50%; transform: translateX(-50%);
    width: min(1000px, calc(100vw - 80px)); background: var(--bg); border: 1px solid var(--line-2);
    border-radius: 16px; display: flex; flex-direction: column; box-shadow: 0 24px 60px rgba(0, 0, 0, 0.5);
  }
  .top { display: flex; justify-content: space-between; align-items: flex-start; padding: 22px 24px 0; }
  h2 { margin: 4px 0 0; font-family: var(--display); font-weight: 700; font-size: 26px; }
  .small { font-size: 12px; }
  .body { flex: 1; min-height: 0; display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 1fr); gap: 20px; padding: 20px 24px; }
  .plan, .log { display: flex; flex-direction: column; gap: 12px; overflow: auto; min-height: 0; }
  .plan p, .log p { margin: 0; }
  .level { display: flex; gap: 14px; align-items: flex-start; }
  .badge { width: 22px; height: 22px; border-radius: 6px; background: var(--text); color: var(--bg); display: grid; place-items: center; font-size: 12px; flex-shrink: 0; margin-top: 10px; }
  .legs { flex: 1; display: flex; flex-direction: column; gap: 6px; }
  .leg { display: flex; align-items: center; gap: 10px; padding: 10px 14px; background: var(--panel); border: 1px solid var(--line); border-radius: 10px; }
  .grow { flex: 1; }
  .note { line-height: 1.5; }
  .log { padding: 16px; background: var(--nav); border: 1px solid var(--line); border-radius: 12px; }
  .event { display: grid; grid-template-columns: 16px 112px minmax(0, 1fr); gap: 10px; align-items: center; }
  .event.past { color: var(--muted); }
  .name { font-size: 13px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .error { margin: 6px 0 0; padding: 10px 12px; border-radius: 8px; background: var(--warn-bg); border: 1px solid #8a4a2a; color: var(--warn-text); font: 12px/1.6 var(--mono); white-space: pre-wrap; }
  .foot { display: flex; justify-content: space-between; align-items: center; gap: 16px; padding: 14px 24px 20px; border-top: 1px solid var(--line); }
  .methods { display: flex; gap: 14px; align-items: center; border: 0; margin: 0; padding: 0; }
  .methods legend { float: left; margin-right: 4px; }
  .method { display: flex; gap: 6px; align-items: center; font-size: 13px; cursor: pointer; }
  .method input { accent-color: var(--accent); }
</style>
