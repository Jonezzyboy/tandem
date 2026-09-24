<script lang="ts">
  import { onDestroy } from 'svelte'
  import { api } from '@lib/api'
  import { ago, reviewLabel } from '@lib/format'
  import { app, fail, log } from '@lib/state.svelte'
  import type { LegView } from '@lib/types'
  import Composer from './Composer.svelte'
  import TrainDialog from './TrainDialog.svelte'
  import Icon from './Icon.svelte'

  let { id }: { id: string } = $props()

  const view = $derived(app.views[id])
  const checks = $derived(app.checks[id] ?? [])
  const activity = $derived(app.activity[id] ?? [])
  const levels = $derived.by(() => {
    const byLevel = new Map<number, LegView[]>()
    for (const l of view?.legs ?? []) byLevel.set(l.level, [...(byLevel.get(l.level) ?? []), l])
    return [...byLevel.entries()].sort((a, b) => a[0] - b[0]).map(([, legs]) => legs)
  })
  const failingChecks = $derived(checks.filter((c) => c.state === 'fail'))
  const checkLegs = $derived([...new Set(checks.map((c) => c.leg))])

  const goEdges = $derived((view?.edges ?? []).filter((e) => e.kind === 'go').length)
  const trainRunning = $derived(app.trains[id]?.running ?? false)

  let syncing = $state(false)
  let pinning = $state(false)
  let checking = $state<string | null>(null)
  let now = $state(Date.now())
  const tick = setInterval(() => (now = Date.now()), 1000)
  onDestroy(() => clearInterval(tick))

  function tone(l: LegView): 'ok' | 'warn' | '' {
    if (l.localError || l.prError || (l.pr && (l.pr.fail > 0 || l.pr.review === 'CHANGES_REQUESTED'))) return 'warn'
    if (l.pr && l.pr.state === 'MERGED') return 'ok'
    if (l.pr && l.pr.review === 'APPROVED' && l.pr.fail === 0 && l.pr.pending === 0) return 'ok'
    return ''
  }

  function localText(l: LegView): { text: string; warn: boolean } {
    if (l.localError) return { text: l.localError, warn: true }
    const parts: string[] = []
    if (l.ahead) parts.push(`↑${l.ahead}`)
    if (l.behind) parts.push(`↓${l.behind}`)
    const detail = l.dirty ? `${l.dirty} uncommitted` : l.behind ? `behind ${l.base}` : l.ahead ? 'clean' : 'no commits yet'
    return { text: [parts.join(' '), detail].filter(Boolean).join(' · '), warn: l.dirty > 0 || l.behind > 0 }
  }

  async function sync() {
    syncing = true
    try {
      const results = await api.sync(id)
      for (const r of results) log(id, `${r.leg}: ${r.message}`, r.ok ? 'muted' : 'warn')
    } catch (e) {
      fail(e)
    } finally {
      syncing = false
    }
  }

  async function pin() {
    pinning = true
    try {
      const results = await api.pin(id)
      for (const r of results) log(id, `Pin ${r.leg} ← ${r.module}: ${r.message}`, r.status === 'pinned' ? 'ok' : r.status === 'already' ? 'muted' : 'warn')
    } catch (e) {
      fail(e)
    } finally {
      pinning = false
    }
  }

  async function runChecks(leg = '') {
    checking = leg || '*'
    app.checks[id] = leg ? checks.filter((c) => c.leg !== leg) : []
    try {
      const results = await api.check(id, leg)
      const failed = results.filter((r) => r.state === 'fail').length
      const passed = results.filter((r) => r.state === 'pass').length
      log(id, `Checks${leg ? ` on ${leg}` : ''}: ${passed} passed, ${failed} failed`, failed ? 'warn' : 'ok')
    } catch (e) {
      fail(e)
    } finally {
      checking = null
    }
  }

  function prState(l: LegView) {
    const pr = l.pr
    if (!pr) return null
    if (pr.state !== 'OPEN') return { icon: 'check' as const, color: 'var(--muted)', text: pr.state.toLowerCase() }
    const total = pr.pass + pr.fail + pr.pending
    if (total === 0) return { icon: 'clock' as const, color: 'var(--muted)', text: 'No checks' }
    if (pr.fail > 0) return { icon: 'x' as const, color: 'var(--warn)', text: (pr.failing ?? []).join(', ') || `${pr.fail} failing` }
    if (pr.pending > 0) return { icon: 'running' as const, color: 'var(--muted)', text: `${pr.pending} of ${total} running` }
    return { icon: 'check' as const, color: 'var(--ok)', text: `${pr.pass}/${total} passing` }
  }

  function reviewIcon(review: string) {
    if (review === 'APPROVED') return { icon: 'check' as const, color: 'var(--ok)' }
    if (review === 'CHANGES_REQUESTED') return { icon: 'alert' as const, color: 'var(--warn)' }
    return { icon: 'clock' as const, color: 'var(--muted)' }
  }
</script>

{#if !view}
  <div class="loading muted">Loading {id}…</div>
{:else}
  <div class="page">
    <header style="--wails-draggable: drag">
      <div class="heading">
        <div class="meta mono">
          <span class="accent">{view.id}</span><span>·</span>
          {#if view.branch !== view.id}<span>branch {view.branch}</span><span>·</span>{/if}
          <span title="Local git state is watched every few seconds; GitHub is read every minute">
            {view.remote ? `GitHub read ${ago(view.remoteAt, now)} ago` : 'GitHub not read yet'}
          </span>
        </div>
        <h1>{view.title || 'Untitled change'}</h1>
      </div>
      <div class="actions" style="--wails-draggable: no-drag">
        <button class="icon-btn" aria-label="Refresh from GitHub" title="Refresh (⌘R)" onclick={() => api.refresh(id)}>
          <Icon name="refresh" />
        </button>
        <button class="btn" onclick={sync} disabled={syncing}>
          <Icon name="sync" spin={syncing} />Sync all
        </button>
        <button class="btn" onclick={() => runChecks()} disabled={checking !== null}>
          <Icon name="play" spin={checking === '*'} />Run checks
        </button>
        <button class="btn" onclick={() => (app.trainOpen = true)} disabled={!view.legs.some((l) => l.pr)}>
          <Icon name="branch" spin={trainRunning} />{trainRunning ? 'Train running' : 'Merge train'}
        </button>
        <button class="btn primary" onclick={() => (app.composer = true)}>
          <Icon name="send" />{view.legs.some((l) => l.pr) ? 'Publish PRs' : 'Open PRs'}
        </button>
      </div>
    </header>

    {#if view.graphError}
      <div class="banner warn">{view.graphError}</div>
    {/if}

    <section class="order" aria-label="Merge order">
      <div class="eyebrow order-label">Merge order</div>
      {#each levels as legs, i}
        {#if i > 0}<div class="arrow muted">→</div>{/if}
        <div class="level">
          {#each legs as l (l.repo)}
            <div class="chip mono {tone(l)}">{l.name}</div>
          {/each}
        </div>
      {/each}
      {#if goEdges > 0}
        <button class="btn small repin" onclick={pin} disabled={pinning} title="Point downstream Go legs at their upstream leg's pushed commit, and commit go.mod/go.sum">
          <Icon name="sync" size={14} spin={pinning} />Re-pin
        </button>
      {/if}
      <div class="order-note muted">
        {#if view.edges.length === 0}
          No dependencies found: legs can merge in any order
        {:else}
          {view.edges.filter((e) => e.via).length} inferred from manifests{#if view.edges.some((e) => !e.via)}, {view.edges.filter((e) => !e.via).length} declared{/if}
        {/if}
      </div>
    </section>

    <section class="legs" aria-label="Legs">
      <div class="row head eyebrow">
        <div>Leg</div><div>Branch · local</div><div>PR</div><div>Checks</div><div>Review</div><div></div>
      </div>
      {#each view.legs as l (l.repo)}
        {@const local = localText(l)}
        {@const ci = prState(l)}
        <div class="row" class:warnrow={tone(l) === 'warn'}>
          <div class="stack">
            <span class="mono strong">{l.repo}</span>
            {#if l.blockers.length}<span class="small warn">{l.blockers.join(' · ')}</span>{:else if view.remote && l.pr}<span class="small ok">ready</span>{/if}
          </div>
          <div class="stack">
            <span class="mono">{view.branch} → {l.base}</span>
            <span class="small" class:warn={local.warn} class:muted={!local.warn}>{local.text}</span>
          </div>
          <div class="stack">
            {#if l.pr}
              <a href={l.pr.url} class="mono" onclick={(e) => { e.preventDefault(); api.openURL(l.pr!.url) }}>#{l.pr.number}</a>
              {#if l.pr.draft}<span class="small muted">draft</span>{/if}
            {:else if l.prError}
              <span class="small warn" title={l.prError}>GitHub error</span>
            {:else}
              <span class="muted">{view.remote ? 'not opened' : '—'}</span>
            {/if}
          </div>
          <div class="cell">
            {#if ci}<Icon name={ci.icon} color={ci.color} /><span class:warn={ci.icon === 'x'}>{ci.text}</span>{:else}<span class="muted">—</span>{/if}
          </div>
          <div class="cell">
            {#if l.pr && l.pr.state === 'OPEN'}
              {@const r = reviewIcon(l.pr.review)}
              <Icon name={r.icon} color={r.color} /><span class:warn={l.pr.review === 'CHANGES_REQUESTED'}>{reviewLabel(l.pr.review)}</span>
            {:else}
              <span class="muted">—</span>
            {/if}
          </div>
          <div class="tools">
            <button class="icon-btn" aria-label="Run checks on {l.name}" title="Run checks" disabled={checking !== null} onclick={() => runChecks(l.name)}>
              <Icon name="play" spin={checking === l.name} />
            </button>
            <button class="icon-btn" aria-label="Open {l.name} in editor" title="Open in editor" onclick={() => api.openEditor(l.worktree).catch(fail)}>
              <Icon name="code" />
            </button>
            <button class="icon-btn" aria-label="Show {l.name} in Finder" title="Show in Finder" onclick={() => api.openFolder(l.worktree).catch(fail)}>
              <Icon name="folder" />
            </button>
          </div>
        </div>
      {/each}
    </section>

    <div class="panels">
      <section class="panel output" aria-label="Local checks">
        <div class="panel-head">
          <div class="panel-title">Local checks</div>
          {#if failingChecks.length}<span class="small warn">{failingChecks.length} failing</span>{/if}
        </div>
        {#if checks.length === 0}
          <p class="muted small">Run checks to see go, php, js and buf results for every leg here.</p>
        {:else}
          <div class="check-list">
            {#each checkLegs as leg (leg)}
              <div class="check-leg mono">{leg}</div>
              {#each checks.filter((c) => c.leg === leg) as c (c.name)}
                <div class="check">
                  {#if c.state === 'running'}<Icon name="running" spin />
                  {:else if c.state === 'pass'}<Icon name="check" color="var(--ok)" />
                  {:else if c.state === 'fail'}<Icon name="x" color="var(--warn)" />
                  {:else}<Icon name="clock" color="var(--muted)" />{/if}
                  <span class="mono grow">{c.name}</span>
                  <span class="small muted">{c.state === 'skip' ? c.output : c.state === 'running' ? 'running' : `${(c.ms / 1000).toFixed(1)}s`}</span>
                </div>
                {#if c.state === 'fail' && c.output}<pre class="log selectable">{c.output}</pre>{/if}
              {/each}
            {/each}
          </div>
        {/if}
      </section>
      <section class="panel activity" aria-label="Activity">
        <div class="panel-head"><div class="panel-title">Activity</div></div>
        {#each activity as a (a.at + a.text)}
          <div class="act"><span class="muted mono">{ago(a.at, now)}</span><span class={a.tone}>{a.text}</span></div>
        {:else}
          <p class="muted small">Syncs, check runs and published PRs from this session show up here.</p>
        {/each}
      </section>
    </div>
  </div>

  {#if app.composer}
    <Composer {view} onclose={() => (app.composer = false)} />
  {/if}
  {#if app.trainOpen}
    <TrainDialog {view} onclose={() => (app.trainOpen = false)} />
  {/if}
{/if}

<style>
  .loading { padding: 80px 40px; }
  .page { padding: 0 32px 28px; display: flex; flex-direction: column; gap: 20px; min-height: 100%; }
  header { display: flex; justify-content: space-between; align-items: flex-end; gap: 24px; padding-top: 28px; }
  .heading { display: flex; flex-direction: column; gap: 6px; min-width: 0; flex: 1; }
  .meta { display: flex; gap: 8px; font-size: 12px; color: var(--muted); white-space: nowrap; overflow: hidden; }
  .meta > :last-child { overflow: hidden; text-overflow: ellipsis; }
  .accent { color: var(--accent-text); }
  h1 { margin: 0; font-family: var(--display); font-weight: 700; font-size: 30px; letter-spacing: -0.01em; line-height: 1.15; }
  .actions { display: flex; gap: 8px; align-items: center; flex-shrink: 0; }
  .banner { padding: 10px 14px; border-radius: 10px; background: var(--warn-bg); border: 1px solid #8a4a2a; font-size: 13px; }

  .order {
    display: flex; align-items: center; gap: 12px; padding: 14px 18px;
    background: var(--panel); border: 1px solid var(--line); border-radius: 12px; flex-wrap: wrap;
  }
  .order-label { width: 92px; }
  .level { display: flex; flex-direction: column; gap: 6px; }
  .chip { padding: 6px 11px; border: 1px solid var(--line-2); border-radius: 8px; font-size: 13px; }
  .chip.ok { border-color: #2f6b4a; background: var(--ok-bg); }
  .chip.warn { border-color: #8a4a2a; background: var(--warn-bg); }
  .order-note { margin-left: auto; font-size: 12px; text-align: right; }
  .repin { margin-left: 8px; }

  .legs { border: 1px solid var(--line); border-radius: 12px; overflow: hidden; }
  .row {
    display: grid;
    grid-template-columns: minmax(0, 2.2fr) minmax(0, 1.6fr) minmax(0, 0.8fr) minmax(0, 1.4fr) minmax(0, 1.3fr) 104px;
    gap: 14px; padding: 13px 16px; align-items: center; border-top: 1px solid var(--line);
  }
  .row.head { border-top: 0; background: var(--panel); padding: 10px 16px; }
  .row.warnrow { background: var(--warn-row); }
  .stack { display: flex; flex-direction: column; gap: 3px; min-width: 0; }
  .stack > * { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .strong { font-size: 13.5px; }
  .small { font-size: 12px; }
  .cell { display: flex; align-items: center; gap: 8px; min-width: 0; }
  .cell span { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .tools { display: flex; justify-content: flex-end; gap: 2px; }
  .tools :global(.icon-btn:disabled) { opacity: 0.4; }

  .panels { display: grid; grid-template-columns: minmax(0, 1fr) 360px; gap: 16px; flex: 1; min-height: 220px; }
  .panel { padding: 16px 18px; border-radius: 12px; border: 1px solid var(--line); display: flex; flex-direction: column; gap: 10px; overflow: auto; }
  .output { background: var(--nav); }
  .activity { background: var(--panel); }
  .panel-head { display: flex; justify-content: space-between; align-items: center; }
  .panel-title { font-weight: 600; }
  .panel p { margin: 0; }
  .check-list { display: flex; flex-direction: column; gap: 6px; }
  .check-leg { font-size: 12px; color: var(--muted); margin-top: 6px; }
  .check-leg:first-child { margin-top: 0; }
  .check { display: flex; align-items: center; gap: 10px; font-size: 13px; }
  .grow { flex: 1; min-width: 0; }
  .log {
    margin: 2px 0 6px 26px; padding: 10px 12px; background: #08090b; border: 1px solid var(--line);
    border-radius: 8px; font: 12px/1.6 var(--mono); color: var(--text-2); white-space: pre-wrap; max-height: 260px; overflow: auto;
  }
  .act { display: grid; grid-template-columns: 34px 1fr; gap: 8px; font-size: 13px; line-height: 1.45; }
</style>
