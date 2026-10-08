<script lang="ts">
  import { onDestroy, onMount } from 'svelte'
  import { api } from '@lib/api'
  import { parseTicket } from '@lib/jira'
  import { navigate } from '@lib/state.svelte'
  import { isRetest, lastTest, loadQueue, loadState, queueCI, tester, type QueueCI, type QueueSort, type QueueView } from '@lib/tester.svelte'
  import { ago, shortRepo } from '@lib/format'
  import type { QueueItem } from '@lib/types'
  import Icon from './Icon.svelte'

  // Jira is reread every two minutes while the page is open.
  const AUTO_REFRESH = 120_000

  onMount(() => { loadQueue(false); loadState() })
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
    switch (queueCI(item)) {
      case 'loading': return { text: 'finding PRs…', tone: 'muted' }
      case 'none': return { text: 'no open PRs: may already be on main', tone: 'muted' }
      case 'failing': {
        const failing = item.prs.filter((p) => p.fail > 0).length
        return { text: `${failing === n && n > 1 ? 'all' : failing} failing CI · not ready`, tone: 'warn' }
      }
      case 'running': return { text: 'CI running', tone: 'muted' }
      case 'passing': return { text: n === 1 ? 'CI passing' : `CI passing on all ${n}`, tone: 'ok' }
    }
  }

  const URGENT = ['highest', 'blocker', 'critical', 'high']
  function priorityRank(p: string): number {
    const i = ['blocker', 'critical', 'highest', 'high', 'medium', 'low', 'lowest'].indexOf(p.toLowerCase())
    return i < 0 ? 4 : i
  }
  const since = (i: QueueItem) => Date.parse(i.statusSince || i.updated) || 0

  function waiting(item: QueueItem): string {
    if (item.statusSince) return `Waiting ${ago(item.statusSince, now)} in ${item.status}`
    return item.updated ? `Updated ${ago(item.updated, now)} ago` : ''
  }

  function retestNote(key: string): string {
    const r = lastTest(key)
    return r ? `Retest · you failed it ${ago(r.at, now)} ago` : ''
  }

  const f = tester.filters
  const VIEWS: { id: QueueView; label: string }[] = [
    { id: 'all', label: 'All' },
    { id: 'ready', label: 'Ready to go' },
    { id: 'retests', label: 'Retests' },
  ]
  const CI_FILTERS: { id: QueueCI | 'all'; label: string }[] = [
    { id: 'all', label: 'CI: any' },
    { id: 'passing', label: 'CI passing' },
    { id: 'failing', label: 'CI failing' },
    { id: 'running', label: 'CI running' },
    { id: 'none', label: 'No PRs' },
  ]
  const SORTS: { id: QueueSort; label: string }[] = [
    { id: 'waiting', label: 'Waiting longest' },
    { id: 'priority', label: 'Priority' },
    { id: 'updated', label: 'Recently updated' },
  ]
  const UNASSIGNED = 'Unassigned'

  const items = $derived(tester.queue?.items ?? [])
  const statuses = $derived.by(() => {
    const q = tester.queue
    if (!q) return []
    return [...new Set([...q.statuses, ...q.items.map((i) => i.status).filter((s) => !q.statuses.includes(s))])]
  })
  const repos = $derived([...new Set(items.flatMap((i) => i.prs.map((p) => p.repo)))].sort())
  const assignees = $derived([...new Set(items.map((i) => i.assignee || UNASSIGNED))].sort())

  function inView(i: QueueItem, view: QueueView): boolean {
    if (view === 'ready') return queueCI(i) === 'passing'
    if (view === 'retests') return isRetest(i.key)
    return true
  }
  const counts = $derived(Object.fromEntries(VIEWS.map((v) => [v.id, items.filter((i) => inView(i, v.id)).length])))

  const filtering = $derived(!!(f.text.trim() || f.view !== 'all' || f.ci !== 'all' || f.status || f.repo || f.assignee))
  function clearFilters() {
    Object.assign(f, { view: 'all', text: '', ci: 'all', status: '', repo: '', assignee: '' })
  }

  const shown = $derived.by(() => {
    const text = f.text.trim().toLowerCase()
    const out = items.filter((i) =>
      inView(i, f.view) &&
      (!text || `${i.key} ${i.summary} ${i.assignee}`.toLowerCase().includes(text)) &&
      // PRs still loading count as a match, so a ticket doesn't blink out and back.
      (f.ci === 'all' || queueCI(i) === f.ci || queueCI(i) === 'loading') &&
      (!f.status || i.status === f.status) &&
      (!f.repo || !i.prsLoaded || i.prs.some((p) => p.repo === f.repo)) &&
      (!f.assignee || (i.assignee || UNASSIGNED) === f.assignee))
    if (f.sort === 'waiting') out.sort((a, b) => since(a) - since(b))
    else if (f.sort === 'updated') out.sort((a, b) => (Date.parse(b.updated) || 0) - (Date.parse(a.updated) || 0))
    else out.sort((a, b) => priorityRank(a.priority) - priorityRank(b.priority) || since(a) - since(b))
    return out
  })

  // Grouped by status, in the order the statuses are listed in Settings.
  const groups = $derived(
    statuses.map((status) => ({ status, items: shown.filter((i) => i.status === status) })).filter((g) => g.items.length),
  )
  // The one ticket to suggest: the first shown whose CI passes.
  const suggested = $derived(tester.state.current ? '' : (groups.flatMap((g) => g.items).find((i) => queueCI(i) === 'passing')?.key ?? ''))

  const busy = $derived(tester.queueLoading || !!tester.queue?.loading)
  const done = $derived(tester.finished)
</script>

<div class="page">
  <header style="--wails-draggable: drag">
    <div class="heading">
      <div class="mono muted small status" role="status">
        {#if tester.queueLoading}Reading Jira…
        {:else if tester.queue?.loading}Finding PRs on GitHub…
        {:else if tester.queue?.at}{items.length} ticket{items.length === 1 ? '' : 's'}{tester.queue.statuses.length ? ` in ${tester.queue.statuses.join(', ')}` : ''} · updated {ago(tester.queue.at, now)} ago
        {:else}Tickets ready to test, with their PRs on GitHub{/if}
        <button class="icon-btn tiny" style="--wails-draggable: no-drag" disabled={busy} onclick={() => loadQueue(true)} aria-label="Refresh now" title="Refresh now · rereads every 2 minutes">
          <Icon name={busy ? 'running' : 'refresh'} size={13} spin={busy} />
        </button>
      </div>
      <h1>Ready to test</h1>
    </div>
    <form class="find" style="--wails-draggable: no-drag" onsubmit={(e) => { e.preventDefault(); open() }}>
      <Icon name="link" />
      <input class="input mono" bind:value={find} placeholder="Paste a Jira link, or type DEV-123" aria-label="Find a change to test" />
    </form>
  </header>
  {#if findError}<div class="small warn">{findError}</div>{/if}
  <div class="bar" class:on={busy} aria-hidden="true"><div></div></div>

  {#if items.length}
    <div class="filters" role="search" aria-label="Filter tickets">
      <div class="search">
        <Icon name="search" size={14} />
        <input class="input" bind:value={f.text} placeholder="Filter key, title, person" aria-label="Filter by key, title or person" />
      </div>
      <div class="seg" role="group" aria-label="Quick views">
        {#each VIEWS as v (v.id)}
          <button class:on={f.view === v.id} aria-pressed={f.view === v.id} onclick={() => (f.view = v.id)}>{v.label} <span class="mono">{counts[v.id]}</span></button>
        {/each}
      </div>
      <select class="input" bind:value={f.ci} aria-label="CI">
        {#each CI_FILTERS as c (c.id)}<option value={c.id}>{c.label}</option>{/each}
      </select>
      {#if statuses.length > 1}
        <select class="input" bind:value={f.status} aria-label="Status">
          <option value="">Status: any</option>
          {#each statuses as s (s)}<option value={s}>{s}</option>{/each}
        </select>
      {/if}
      {#if repos.length > 1}
        <select class="input" bind:value={f.repo} aria-label="Repo">
          <option value="">Repo: any</option>
          {#each repos as r (r)}<option value={r}>{shortRepo(r)}</option>{/each}
        </select>
      {/if}
      {#if assignees.length > 1}
        <select class="input" bind:value={f.assignee} aria-label="Assignee">
          <option value="">Assignee: any</option>
          {#each assignees as a (a)}<option value={a}>{a}</option>{/each}
        </select>
      {/if}
      {#if filtering}
        <span class="small muted">{shown.length} of {items.length}</span>
        <button class="btn small" onclick={clearFilters}>Clear</button>
      {/if}
      <span class="grow"></span>
      <label class="sort small muted">Sort
        <select class="input" bind:value={f.sort}>
          {#each SORTS as s (s.id)}<option value={s.id}>{s.label}</option>{/each}
        </select>
      </label>
    </div>
  {/if}

  <div class="scroll">
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
    <section class="group" aria-label="Loading">
      {#each [0, 1, 2] as i (i)}<div class="row skeleton"></div>{/each}
    </section>
  {:else if !tester.queue.setup && !tester.queue.error && items.length === 0}
    <p class="muted">No tickets in {tester.queue.statuses.join(' or ')} right now. You can still open any ticket above.</p>
  {:else if filtering && shown.length === 0}
    <p class="muted">No tickets match these filters. <button class="link" onclick={clearFilters}>Clear filters</button></p>
  {/if}

  {#each groups as g (g.status)}
    <section class="group" aria-label={g.status}>
      <div class="group-head eyebrow"><span>{g.status} · {g.items.length}</span><span>PRs · CI</span></div>
      {#each g.items as item (item.key)}
        {@const c = ci(item)}
        {@const testing = tester.state.current?.key === item.key}
        {@const retest = retestNote(item.key)}
        <article class="row" class:testing>
          <div class="stack grow">
            <div class="meta small">
              <button class="link mono" onclick={() => api.openURL(item.url)}>{item.key}</button>
              {#if item.priority}<span class:urgent={URGENT.includes(item.priority.toLowerCase())} class:muted={!URGENT.includes(item.priority.toLowerCase())}>{item.priority}</span>{/if}
              <span class="muted">{[item.type, item.assignee || UNASSIGNED].filter(Boolean).join(' · ')}</span>
              {#if retest}<span class="tag warn-tag">{retest}</span>{/if}
            </div>
            <div class="title">{item.summary}</div>
            <div class="small muted">{waiting(item)}</div>
          </div>
          <div class="prs">
            {#if item.prs.length}
              <div class="chips">
                {#each item.prs as p (p.url)}
                  <button class="chip mono" onclick={() => api.openURL(p.url)} title="Open the PR on GitHub">{shortRepo(p.repo)} #{p.number}</button>
                {/each}
              </div>
            {/if}
            <span class="small {c.tone}">{c.text}</span>
          </div>
          <div class="act">
            <button class="btn small" class:primary={testing || item.key === suggested} onclick={() => navigate({ name: 'test', id: item.key })}>
              {testing ? 'Testing now →' : isRetest(item.key) ? 'Retest' : 'Test'}
            </button>
          </div>
        </article>
      {/each}
    </section>
  {/each}
  </div>
</div>

<style>
  /* The header stays put; only the list below it scrolls. */
  .page { height: 100%; box-sizing: border-box; padding: 0 32px; display: flex; flex-direction: column; gap: 14px; overflow: hidden; }
  .page > :not(.scroll) { flex-shrink: 0; }
  .scroll { flex: 1; min-height: 0; overflow-y: auto; margin: 0 -32px; padding: 0 32px 32px; display: flex; flex-direction: column; gap: 16px; }
  header { padding-top: 28px; display: flex; justify-content: space-between; align-items: flex-end; gap: 24px; }
  .heading { display: flex; flex-direction: column; gap: 4px; min-width: 0; }
  .status { display: flex; align-items: center; gap: 6px; min-height: 22px; }
  .tiny { width: 22px; height: 22px; }
  h1 { margin: 0; font-family: var(--display); font-weight: 700; font-size: 30px; line-height: 1.15; }
  .small { font-size: 12px; }
  .grow { flex: 1; min-width: 0; }
  .stack { display: flex; flex-direction: column; gap: 5px; }
  .strong { font-weight: 600; font-size: 15px; }
  .sub { color: var(--text-2); }
  .find { position: relative; width: 320px; flex-shrink: 0; }
  .find :global(svg) { position: absolute; left: 12px; top: 50%; transform: translateY(-50%); color: var(--muted); pointer-events: none; }
  .find .input { width: 100%; padding-left: 36px; font-size: 13px; }
  /* A thin sweep under the header while Jira or GitHub is being read. */
  .bar { height: 2px; margin: -8px 0 -6px; border-radius: 1px; overflow: hidden; background: transparent; }
  .bar.on { background: var(--line); }
  .bar > div { width: 30%; height: 100%; background: var(--accent-text); transform: translateX(-100%); }
  .bar.on > div { animation: sweep 1.1s ease-in-out infinite; }
  @keyframes sweep { to { transform: translateX(340%); } }
  .filters { display: flex; flex-wrap: wrap; gap: 8px; align-items: center; }
  .filters .input { min-height: 30px; font-size: 13px; }
  .filters select.input { max-width: 190px; }
  .search { position: relative; flex: 1; min-width: 190px; max-width: 240px; }
  .search :global(svg) { position: absolute; left: 10px; top: 50%; transform: translateY(-50%); color: var(--muted); pointer-events: none; }
  .search .input { width: 100%; padding-left: 30px; }
  .seg { display: inline-flex; padding: 2px; gap: 2px; border-radius: 8px; border: 1px solid var(--line-2); background: var(--nav); }
  .seg button { min-height: 24px; padding: 0 10px; border: 0; border-radius: 6px; background: transparent; color: var(--text-2); font-size: 12px; cursor: pointer; white-space: nowrap; }
  .seg button .mono { color: var(--muted); margin-left: 2px; }
  .seg button:hover { color: var(--text); }
  .seg button.on { background: var(--raised); color: var(--text); box-shadow: 0 0 0 1px var(--line-2); }
  .sort { display: flex; align-items: center; gap: 8px; }
  .note { display: flex; gap: 12px; align-items: center; padding: 10px 12px 10px 14px; border-radius: 10px; background: var(--panel); border: 1px solid var(--line-2); font-size: 13px; }
  .done { display: flex; flex-direction: column; gap: 12px; padding: 16px 18px; border-radius: 12px; background: var(--ok-bg); border: 1px solid var(--ok-border); }
  .done.bad { background: var(--warn-row); border-color: var(--warn-border); }
  .done-head { display: flex; justify-content: space-between; align-items: flex-start; gap: 16px; }
  .done ul { margin: 0; padding: 0; list-style: none; display: flex; flex-direction: column; gap: 8px; font-size: 13px; }
  .done li { display: grid; grid-template-columns: 16px minmax(0, 1fr); gap: 10px; align-items: start; }
  p { margin: 0; }
  .group { border: 1px solid var(--line); border-radius: 12px; overflow: hidden; flex-shrink: 0; }
  .group-head { display: flex; justify-content: space-between; padding: 9px 16px; background: var(--panel); }
  .group-head span:last-child { width: 240px; }
  .row { display: grid; grid-template-columns: minmax(0, 1fr) 240px 110px; gap: 20px; align-items: center; padding: 12px 16px; border-top: 1px solid var(--line); }
  .group-head + .row { border-top: 0; }
  .row.testing { background: var(--ok-bg); }
  .row.skeleton { height: 72px; padding: 0; border-top: 1px solid var(--line); animation: pulse 1.4s ease-in-out infinite; }
  .row.skeleton:first-child { border-top: 0; }
  @keyframes pulse { 50% { opacity: 0.5; } }
  .meta { display: flex; gap: 8px; align-items: center; flex-wrap: wrap; }
  .urgent { color: var(--warn-text); }
  .tag { font-size: 11.5px; padding: 1px 8px; border-radius: 999px; white-space: nowrap; }
  .warn-tag { color: var(--warn-text); background: var(--warn-bg); border: 1px solid var(--warn-border); }
  .title { font-size: 15px; font-weight: 500; }
  .prs { display: flex; flex-direction: column; gap: 5px; min-width: 0; }
  .chips { display: flex; gap: 6px; flex-wrap: wrap; }
  .chip { padding: 2px 7px; border-radius: 6px; border: 1px solid var(--line-2); background: var(--raised); font-size: 11.5px; color: var(--text-2); cursor: pointer; }
  .chip:hover { color: var(--text); }
  .act { display: flex; justify-content: flex-end; }
  .ok { color: var(--ok-text); }
  .link { border: 0; background: none; padding: 0; color: var(--accent-text); cursor: pointer; }
  .link:hover { color: var(--link-hover); }
</style>
