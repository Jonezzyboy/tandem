<script lang="ts">
  import { onDestroy, onMount } from 'svelte'
  import { api } from '@lib/api'
  import { parseTicket } from '@lib/jira'
  import { navigate } from '@lib/state.svelte'
  import { loadQueue, tester } from '@lib/tester.svelte'
  import { ago } from '@lib/format'
  import type { QueueItem } from '@lib/types'
  import Icon from './Icon.svelte'

  // Jira is reread every two minutes while the page is open.
  const AUTO_REFRESH = 120_000

  onMount(() => { loadQueue(false) })
  let now = $state(Date.now())
  const tick = setInterval(() => {
    now = Date.now()
    const at = tester.queue?.at ? Date.parse(tester.queue.at) : 0
    if (!tester.queueLoading && !tester.queue?.loading && now - at > AUTO_REFRESH) loadQueue(true)
  }, 5_000)
  onDestroy(() => clearInterval(tick))

  let find = $state('')
  let findError = $state('')
  function open() {
    const text = find.trim()
    const key = parseTicket(text)?.key ?? text.toUpperCase()
    if (!/^[A-Z][A-Z0-9_]+-[0-9]+$/.test(key)) {
      findError = 'Paste a Jira link, or type a key like DEV-123'
      return
    }
    find = findError = ''
    navigate({ name: 'test', id: key })
  }

  function ci(item: QueueItem): { text: string; tone: 'ok' | 'warn' | 'muted' } {
    const n = item.prs.length
    if (!item.prsLoaded) return { text: 'finding PRs…', tone: 'muted' }
    if (n === 0) return { text: 'no open PRs found', tone: 'muted' }
    const failing = item.prs.filter((p) => p.fail > 0).length
    if (failing) return { text: `${failing} PR${failing === 1 ? '' : 's'} failing CI`, tone: 'warn' }
    if (item.prs.some((p) => p.pending > 0)) return { text: 'CI running', tone: 'muted' }
    return { text: `CI passing on ${n === 1 ? 'its PR' : `all ${n}`}`, tone: 'ok' }
  }

  function byline(item: QueueItem): string {
    return [item.assignee, item.updated ? `updated ${ago(item.updated, now)} ago` : ''].filter(Boolean).join(' · ')
  }

  // Grouped by status, in the order the statuses are listed in Settings.
  const groups = $derived.by(() => {
    const q = tester.queue
    if (!q) return []
    const order = [...q.statuses, ...q.items.map((i) => i.status).filter((s) => !q.statuses.includes(s))]
    return [...new Set(order)]
      .map((status) => ({ status, items: q.items.filter((i) => i.status === status) }))
      .filter((g) => g.items.length)
  })

  const busy = $derived(tester.queueLoading || !!tester.queue?.loading)
  const done = $derived(tester.finished)
</script>

<div class="page">
  <header style="--wails-draggable: drag">
    <div class="heading">
      <div class="mono muted small">
        {#if tester.queue?.statuses.length}Tickets in {tester.queue.statuses.join(', ')}{:else}Tickets ready to test{/if}, with their PRs on GitHub
      </div>
      <h1>Ready to test</h1>
    </div>
    <form class="find" style="--wails-draggable: no-drag" onsubmit={(e) => { e.preventDefault(); open() }}>
      <Icon name="link" />
      <input class="input mono" bind:value={find} placeholder="Paste a Jira link, or type DEV-123" aria-label="Find a change to test" />
    </form>
  </header>
  {#if findError}<div class="small warn">{findError}</div>{/if}

  <div class="refresh-row">
    <span class="small muted" role="status">
      {#if tester.queueLoading}Reading Jira…
      {:else if tester.queue?.loading}Finding PRs on GitHub…
      {:else if tester.queue?.at}Updated {ago(tester.queue.at, now)} ago · refreshes every 2 minutes
      {/if}
    </span>
    <button class="btn small" disabled={busy} onclick={() => loadQueue(true)}>
      <Icon name={busy ? 'running' : 'refresh'} size={14} spin={busy} />{busy ? 'Refreshing' : 'Refresh'}
    </button>
  </div>
  <div class="bar" class:on={busy} aria-hidden="true"><div></div></div>

  {#if done}
    <section class="done" class:bad={!done.outcome.done} aria-label="Testing finished">
      <div class="done-head">
        <div class="stack">
          <span class="strong">{done.outcome.done ? `Every repo is back on main` : `Some repos are still on ${done.key}`}</span>
          {#if done.verdict}<span class="small sub">{done.verdict}.</span>{/if}
        </div>
        <button class="btn small" onclick={() => (tester.finished = null)}>Dismiss</button>
      </div>
      <ul>
        {#each done.outcome.steps as s (s.repo)}
          <li>
            <Icon name={s.ok ? 'check' : 'x'} color={s.ok ? 'var(--ok)' : 'var(--warn)'} />
            <span><span class="mono">{s.repo}</span> <span class:sub={s.ok} class:warn={!s.ok}>{s.message}</span></span>
          </li>
        {/each}
      </ul>
    </section>
  {/if}

  {#if tester.queue?.setup}
    <div class="note">
      <Icon name="alert" color="var(--warn)" />
      <span class="grow">{tester.queue.setup}</span>
      <button class="btn small" onclick={() => navigate({ name: 'settings' })}>Open Settings</button>
    </div>
  {:else if tester.queue?.error}
    <div class="note"><Icon name="alert" color="var(--warn)" /><span class="grow warn selectable">Couldn't read Jira: {tester.queue.error}</span></div>
  {/if}
  {#if tester.queue?.prError}
    <div class="note"><Icon name="alert" color="var(--warn)" /><span class="grow warn selectable">{tester.queue.prError}</span></div>
  {/if}

  {#if !tester.queue}
    <section class="list" aria-label="Loading">
      {#each [0, 1, 2] as i (i)}<div class="card skeleton"></div>{/each}
    </section>
  {:else if !tester.queue.setup && !tester.queue.error && tester.queue.items.length === 0}
    <p class="muted">No tickets in {tester.queue.statuses.join(' or ')} right now. You can still open any ticket above.</p>
  {/if}

  {#each groups as g (g.status)}
    <section class="list" aria-label={g.status}>
      <div class="eyebrow">{g.status} · {g.items.length}</div>
      {#each g.items as item (item.key)}
        {@const c = ci(item)}
        {@const testing = tester.state.current?.key === item.key}
        <article class="card" class:testing>
          <div class="stack grow">
            <div class="meta small">
              <button class="link mono" onclick={() => api.openURL(item.url)}>{item.key}</button>
              <span class="muted">{byline(item)}</span>
            </div>
            <div class="title">{item.summary}</div>
            <div class="prs">
              {#each item.prs as p (p.url)}
                <button class="chip mono" onclick={() => api.openURL(p.url)} title="Open the PR on GitHub">{p.repo.split('/')[1]} #{p.number}</button>
              {/each}
              <span class="small {c.tone}">{c.text}</span>
            </div>
          </div>
          <button class="btn" class:primary={!testing} onclick={() => navigate({ name: 'test', id: item.key })}>
            {testing ? 'Testing now →' : 'Test this change'}
          </button>
        </article>
      {/each}
    </section>
  {/each}
</div>

<style>
  .page { padding: 0 32px 32px; display: flex; flex-direction: column; gap: 16px; }
  header { padding-top: 28px; display: flex; justify-content: space-between; align-items: flex-end; gap: 24px; }
  .heading { display: flex; flex-direction: column; gap: 4px; min-width: 0; }
  h1 { margin: 0; font-family: var(--display); font-weight: 700; font-size: 30px; line-height: 1.15; }
  .small { font-size: 12px; }
  .grow { flex: 1; min-width: 0; }
  .stack { display: flex; flex-direction: column; gap: 6px; }
  .strong { font-weight: 600; font-size: 15px; }
  .sub { color: var(--text-2); }
  .find { position: relative; width: 340px; flex-shrink: 0; }
  .find :global(svg) { position: absolute; left: 12px; top: 50%; transform: translateY(-50%); color: var(--muted); pointer-events: none; }
  .find .input { width: 100%; padding-left: 36px; font-size: 13px; }
  .refresh-row { display: flex; justify-content: space-between; align-items: center; gap: 12px; margin-bottom: -10px; }
  /* A thin sweep under the header while Jira or GitHub is being read. */
  .bar { height: 2px; border-radius: 1px; overflow: hidden; background: transparent; }
  .bar.on { background: var(--line); }
  .bar > div { width: 30%; height: 100%; background: var(--accent-text); transform: translateX(-100%); }
  .bar.on > div { animation: sweep 1.1s ease-in-out infinite; }
  @keyframes sweep { to { transform: translateX(340%); } }
  .note { display: flex; gap: 12px; align-items: center; padding: 10px 12px 10px 14px; border-radius: 10px; background: var(--panel); border: 1px solid var(--line-2); font-size: 13px; }
  .done { display: flex; flex-direction: column; gap: 12px; padding: 16px 18px; border-radius: 12px; background: var(--ok-bg); border: 1px solid var(--ok-border); }
  .done.bad { background: var(--warn-row); border-color: var(--warn-border); }
  .done-head { display: flex; justify-content: space-between; align-items: flex-start; gap: 16px; }
  .done ul { margin: 0; padding: 0; list-style: none; display: flex; flex-direction: column; gap: 8px; font-size: 13px; }
  .done li { display: grid; grid-template-columns: 16px minmax(0, 1fr); gap: 10px; align-items: start; }
  p { margin: 0; }
  .list { display: flex; flex-direction: column; gap: 10px; }
  .card { display: flex; gap: 16px; align-items: center; padding: 16px 18px; background: var(--panel); border: 1px solid var(--line); border-radius: 12px; }
  .card.testing { border-color: var(--ok-border); }
  .card.skeleton { height: 96px; padding: 0; animation: pulse 1.4s ease-in-out infinite; }
  @keyframes pulse { 50% { opacity: 0.5; } }
  .meta { display: flex; gap: 10px; align-items: center; flex-wrap: wrap; }
  .title { font-size: 16px; font-weight: 500; }
  .prs { display: flex; gap: 6px; flex-wrap: wrap; align-items: center; }
  .chip { padding: 3px 8px; border-radius: 6px; border: 1px solid var(--line-2); background: var(--raised); font-size: 12px; color: var(--text-2); cursor: pointer; }
  .chip:hover { color: var(--text); }
  .ok { color: var(--ok-text); }
  .link { border: 0; background: none; padding: 0; color: var(--accent-text); cursor: pointer; }
  .link:hover { color: var(--link-hover); }
</style>
