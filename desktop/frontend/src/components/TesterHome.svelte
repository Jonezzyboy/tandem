<script lang="ts">
  import { onMount } from 'svelte'
  import { api } from '@lib/api'
  import { parseTicket } from '@lib/jira'
  import { navigate } from '@lib/state.svelte'
  import { prefs } from '@lib/settings.svelte'
  import { loadQueue, tester } from '@lib/tester.svelte'
  import { ago } from '@lib/format'
  import type { QueueItem } from '@lib/types'
  import Icon from './Icon.svelte'

  onMount(() => { loadQueue(false) })

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
    if (n === 0) return { text: 'no open PRs found', tone: 'muted' }
    const failing = item.prs.filter((p) => p.fail > 0).length
    if (failing) return { text: `${failing} PR${failing === 1 ? '' : 's'} failing CI`, tone: 'warn' }
    if (item.prs.some((p) => p.pending > 0)) return { text: 'CI running', tone: 'muted' }
    return { text: `CI passing on ${n === 1 ? 'its PR' : `all ${n}`}`, tone: 'ok' }
  }

  function pushed(item: QueueItem): string {
    const latest = item.prs.map((p) => p.updated).sort().pop()
    const who = item.prs.find((p) => p.updated === latest)?.author || item.assignee
    return [who, latest ? `updated ${ago(latest)} ago` : ''].filter(Boolean).join(' · ')
  }

  const done = $derived(tester.finished)
</script>

<div class="page">
  <header style="--wails-draggable: drag">
    <div class="heading">
      <div class="mono muted small">Tickets in {prefs.settings.tester.readyStatus}, with their PRs on GitHub</div>
      <h1>Ready to test</h1>
    </div>
    <div class="tools" style="--wails-draggable: no-drag">
      <form class="find" onsubmit={(e) => { e.preventDefault(); open() }}>
        <Icon name="link" />
        <input class="input mono" bind:value={find} placeholder="Paste a Jira link, or type DEV-123" aria-label="Find a change to test" />
      </form>
      <button class="icon-btn" aria-label="Refresh" title="Refresh" disabled={tester.queueLoading} onclick={() => loadQueue(true)}>
        <Icon name={tester.queueLoading ? 'running' : 'refresh'} spin={tester.queueLoading} />
      </button>
    </div>
  </header>
  {#if findError}<div class="small warn">{findError}</div>{/if}

  {#if done}
    <section class="done" class:bad={done.result === 'failed' || !done.outcome.done} aria-label="Testing finished">
      <div class="done-head">
        <div class="stack">
          <span class="strong">
            {done.key}
            {done.result === 'passed' ? 'passed' : done.result === 'failed' ? 'failed' : 'testing stopped'}
            {done.outcome.done ? 'and every repo is back on main' : ', but some repos are still on its branch'}
          </span>
          {#if done.outcome.jira}<span class="small sub">Jira {done.outcome.jira}.</span>{/if}
          {#if done.outcome.jiraError}<span class="small warn">Jira wasn't updated: {done.outcome.jiraError}</span>{/if}
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
    <div class="note"><Icon name="alert" color="var(--warn)" /><span class="grow warn selectable">{tester.queue.error}</span></div>
  {/if}

  <section class="list" aria-label="Ready to test">
    {#if !tester.queue}
      <p class="muted">Reading Jira and GitHub…</p>
    {:else if !tester.queue.setup && !tester.queue.error && tester.queue.items.length === 0}
      <p class="muted">Nothing is in {prefs.settings.tester.readyStatus} right now. You can still open any ticket above.</p>
    {/if}
    {#each tester.queue?.items ?? [] as item (item.key)}
      {@const c = ci(item)}
      {@const testing = tester.state.current?.key === item.key}
      <article class="card" class:testing>
        <div class="stack grow">
          <div class="meta small">
            <button class="link mono" onclick={() => api.openURL(item.url)}>{item.key}</button>
            <span class="status mono">{item.status}</span>
            <span class="muted">{pushed(item)}</span>
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
</div>

<style>
  .page { padding: 0 32px 32px; display: flex; flex-direction: column; gap: 18px; }
  header { padding-top: 28px; display: flex; justify-content: space-between; align-items: flex-end; gap: 24px; }
  .heading { display: flex; flex-direction: column; gap: 4px; }
  h1 { margin: 0; font-family: var(--display); font-weight: 700; font-size: 30px; line-height: 1.15; }
  .small { font-size: 12px; }
  .grow { flex: 1; min-width: 0; }
  .stack { display: flex; flex-direction: column; gap: 6px; }
  .strong { font-weight: 600; font-size: 15px; }
  .sub { color: var(--text-2); }
  .tools { display: flex; gap: 6px; align-items: center; }
  .find { position: relative; width: 340px; }
  .find :global(svg) { position: absolute; left: 12px; top: 50%; transform: translateY(-50%); color: var(--muted); pointer-events: none; }
  .find .input { width: 100%; padding-left: 36px; font-size: 13px; }
  .note { display: flex; gap: 12px; align-items: center; padding: 10px 12px 10px 14px; border-radius: 10px; background: var(--panel); border: 1px solid var(--line-2); font-size: 13px; }
  .done { display: flex; flex-direction: column; gap: 12px; padding: 16px 18px; border-radius: 12px; background: var(--ok-bg); border: 1px solid var(--ok-border); }
  .done.bad { background: var(--warn-row); border-color: var(--warn-border); }
  .done-head { display: flex; justify-content: space-between; align-items: flex-start; gap: 16px; }
  .done ul { margin: 0; padding: 0; list-style: none; display: flex; flex-direction: column; gap: 8px; font-size: 13px; }
  .done li { display: grid; grid-template-columns: 16px minmax(0, 1fr); gap: 10px; align-items: start; }
  .list { display: flex; flex-direction: column; gap: 10px; }
  .list p { margin: 0; }
  .card { display: flex; gap: 16px; align-items: center; padding: 16px 18px; background: var(--panel); border: 1px solid var(--line); border-radius: 12px; }
  .card.testing { border-color: var(--ok-border); }
  .meta { display: flex; gap: 10px; align-items: center; flex-wrap: wrap; }
  .status { padding: 1px 7px; border-radius: 6px; color: var(--ok-text); background: var(--ok-bg); border: 1px solid var(--ok-border); font-size: 12px; }
  .title { font-size: 16px; font-weight: 500; }
  .prs { display: flex; gap: 6px; flex-wrap: wrap; align-items: center; }
  .chip { padding: 3px 8px; border-radius: 6px; border: 1px solid var(--line-2); background: var(--raised); font-size: 12px; color: var(--text-2); cursor: pointer; }
  .chip:hover { color: var(--text); }
  .ok { color: var(--ok-text); }
  .link { border: 0; background: none; padding: 0; color: var(--accent-text); cursor: pointer; }
  .link:hover { color: var(--link-hover); }
</style>
