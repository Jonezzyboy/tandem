<script lang="ts">
  import { onMount } from 'svelte'
  import { api } from '@lib/api'
  import { ago } from '@lib/format'
  import { navigate } from '@lib/state.svelte'
  import { loadQueue, loadState, tester } from '@lib/tester.svelte'
  import type { TestRecord } from '@lib/types'

  onMount(() => {
    loadState()
    if (!tester.queue) loadQueue(false)
  })

  const WEEK = 7 * 24 * 3600_000
  const history = $derived(tester.state.history)
  const queued = $derived(new Set((tester.queue?.items ?? []).map((i) => i.key)))
  // The newest record of each ticket you failed that is back in the queue.
  const retests = $derived(new Set(history.filter((r, i) => r.result === 'failed' && queued.has(r.key) && history.findIndex((x) => x.key === r.key) === i)))

  type Filter = 'all' | 'passed' | 'failed' | 'retest'
  let filter = $state<Filter>('all')
  const FILTERS: { id: Filter; label: string }[] = [
    { id: 'all', label: 'All' },
    { id: 'passed', label: 'Passed' },
    { id: 'failed', label: 'Failed' },
    { id: 'retest', label: 'Back for retest' },
  ]
  const shown = $derived(history.filter((r) => filter === 'all' || (filter === 'retest' ? retests.has(r) : r.result === filter)))

  function minutes(r: TestRecord): number | null {
    const start = r.started ? Date.parse(r.started) : 0
    return start > 0 ? Math.max(1, Math.round((Date.parse(r.at) - start) / 60_000)) : null
  }
  function duration(m: number): string {
    return m < 60 ? `${m}m` : `${Math.floor(m / 60)}h ${m % 60}m`
  }

  const stats = $derived.by(() => {
    const week = history.filter((r) => Date.now() - Date.parse(r.at) < WEEK)
    const passed = week.filter((r) => r.result === 'passed').length
    const times = week.map(minutes).filter((m): m is number => m !== null).sort((a, b) => a - b)
    return {
      tested: week.length,
      passed,
      passRate: week.length ? Math.round((passed / week.length) * 100) : 0,
      typical: times.length ? duration(times[Math.floor(times.length / 2)]) : '',
    }
  })

  function day(iso: string): string {
    const d = new Date(iso)
    const today = new Date()
    const days = Math.round((new Date(today.toDateString()).getTime() - new Date(d.toDateString()).getTime()) / 86_400_000)
    if (days === 0) return 'Today'
    if (days === 1) return 'Yesterday'
    return d.toLocaleDateString([], days < 7 ? { weekday: 'long' } : { day: 'numeric', month: 'short' })
  }
  const groups = $derived.by(() => {
    const out: { day: string; items: TestRecord[] }[] = []
    for (const r of shown) {
      const d = day(r.at)
      if (out.at(-1)?.day !== d) out.push({ day: d, items: [] })
      out.at(-1)!.items.push(r)
    }
    return out
  })

  function detail(r: TestRecord): string {
    const m = minutes(r)
    return [
      r.checks ? `${r.checked ?? 0} of ${r.checks} checks` : '',
      m ? duration(m) : '',
      new Date(r.at).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }),
    ].filter(Boolean).join(' · ')
  }
</script>

<div class="page">
  <header style="--wails-draggable: drag">
    <div class="heading">
      <div class="mono muted small">The last {history.length || 'few'} changes you tested on this machine</div>
      <h1>Recently tested</h1>
    </div>
    {#if history.length}
      <div class="seg" role="group" aria-label="Result" style="--wails-draggable: no-drag">
        {#each FILTERS as f (f.id)}
          <button class:on={filter === f.id} aria-pressed={filter === f.id} onclick={() => (filter = f.id)}>
            {f.label}{#if f.id === 'retest'} <span class="mono">{retests.size}</span>{/if}
          </button>
        {/each}
      </div>
    {/if}
  </header>
  <div class="scroll">
  {#if history.length}
    <div class="stats">
      <div class="stat"><span class="eyebrow">This week</span><span class="value">{stats.tested}</span><span class="small muted">change{stats.tested === 1 ? '' : 's'} tested</span></div>
      <div class="stat"><span class="eyebrow">Passed</span><span class="value ok">{stats.tested ? `${stats.passRate}%` : '—'}</span><span class="small muted">{stats.passed} of {stats.tested} this week</span></div>
      <div class="stat"><span class="eyebrow">Typical test</span><span class="value">{stats.typical || '—'}</span><span class="small muted">switch to verdict</span></div>
      <div class="stat"><span class="eyebrow">Back for retest</span><span class="value" class:warn={retests.size > 0}>{retests.size}</span><span class="small muted">you failed, now ready again</span></div>
    </div>
  {/if}

  {#each groups as g (g.day)}
    <section class="group" aria-label={g.day}>
      <div class="group-head eyebrow">{g.day}</div>
      {#each g.items as r, i (r.key + r.at + i)}
        {@const retest = retests.has(r)}
        <div class="item" class:retest>
          <span class="result mono {r.result}">{r.result}</span>
          <div class="stack">
            <span>
              {#if r.url}<button class="link mono" onclick={() => api.openURL(r.url)}>{r.key}</button>{:else}<span class="mono">{r.key}</span>{/if}
              <span class="sub">{r.title}</span>
            </span>
            {#if r.note}<span class="small note-text selectable">“{r.note}”</span>{/if}
            <span class="small muted">{detail(r)}</span>
          </div>
          {#if r.to}<span class="small" class:muted={!retest} class:warn={retest}>{retest ? 'Back to test' : r.to}</span>{/if}
          <button class="btn small" class:primary={retest} onclick={() => navigate({ name: 'test', id: r.key })}>{retest ? 'Retest' : 'Open'}</button>
        </div>
      {/each}
    </section>
  {:else}
    <p class="muted">{history.length ? 'Nothing matches this filter.' : 'Changes you test show up here once you go back to main.'}</p>
  {/each}
  </div>
</div>

<style>
  /* The header stays put; only the list below it scrolls. */
  .page { height: 100%; box-sizing: border-box; padding: 0 32px; display: flex; flex-direction: column; gap: 18px; overflow: hidden; }
  .page > :not(.scroll) { flex-shrink: 0; }
  .scroll { flex: 1; min-height: 0; overflow-y: auto; margin: 0 -32px; padding: 0 32px 32px; display: flex; flex-direction: column; gap: 16px; }
  header { padding-top: 28px; display: flex; justify-content: space-between; align-items: flex-end; gap: 24px; }
  .heading { display: flex; flex-direction: column; gap: 4px; }
  h1 { margin: 0; font-family: var(--display); font-weight: 700; font-size: 30px; line-height: 1.15; }
  .small { font-size: 12px; }
  .sub { color: var(--text-2); margin-left: 6px; }
  .seg { display: inline-flex; padding: 2px; gap: 2px; border-radius: 8px; border: 1px solid var(--line-2); background: var(--nav); }
  .seg button { min-height: 26px; padding: 0 10px; border: 0; border-radius: 6px; background: transparent; color: var(--text-2); font-size: 12px; cursor: pointer; white-space: nowrap; }
  .seg button .mono { color: var(--muted); margin-left: 4px; }
  .seg button:hover { color: var(--text); }
  .seg button.on { background: var(--raised); color: var(--text); box-shadow: 0 0 0 1px var(--line-2); }
  .stats { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 12px; flex-shrink: 0; }
  .stat { display: flex; flex-direction: column; gap: 4px; padding: 14px 16px; background: var(--panel); border: 1px solid var(--line); border-radius: 12px; }
  .value { font-family: var(--display); font-weight: 700; font-size: 26px; line-height: 1.1; }
  .value.ok { color: var(--ok-text); }
  p { margin: 0; }
  .group { border: 1px solid var(--line); border-radius: 12px; overflow: hidden; flex-shrink: 0; }
  .group-head { padding: 9px 16px; background: var(--panel); }
  .item { display: flex; gap: 14px; align-items: center; padding: 12px 16px; border-top: 1px solid var(--line); font-size: 14px; }
  .group-head + .item { border-top: 0; }
  .item.retest { background: var(--warn-row); }
  .stack { display: flex; flex-direction: column; gap: 3px; flex: 1; min-width: 0; }
  .note-text { color: var(--text-2); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .result { width: 64px; flex-shrink: 0; text-align: center; font-size: 11px; padding: 2px 0; border-radius: 6px; border: 1px solid var(--line-2); color: var(--muted); }
  .result.passed { color: var(--ok-text); border-color: var(--ok-border); background: var(--ok-bg); }
  .result.moved { color: var(--accent-text); border-color: var(--accent); background: var(--accent-bg); }
  .result.failed { color: var(--warn-text); border-color: var(--warn-border); background: var(--warn-bg); }
  .link { border: 0; background: none; padding: 0; color: var(--accent-text); cursor: pointer; }
  .link:hover { color: var(--link-hover); }
</style>
