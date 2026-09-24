<script lang="ts">
  import { api } from '@lib/api'
  import { ago } from '@lib/format'
  import { app, loadCleanPlan, loadInbox, navigate } from '@lib/state.svelte'
  import type { InboxItem } from '@lib/types'
  import CleanDialog from './CleanDialog.svelte'
  import Icon from './Icon.svelte'

  const review = $derived(app.inbox?.review ?? [])
  const mine = $derived(app.inbox?.mine ?? [])
  const needAction = $derived(app.changes.filter((c) => c.remote && c.blocked > 0))
  const ready = $derived(app.changes.filter((c) => c.remote && c.blocked === 0 && c.legs > 0))

  const cleanable = $derived((app.cleanPlan ?? []).filter((c) => c.ready))
  let cleaning = $state(false)

  function open(it: InboxItem) {
    if (it.changeId) navigate({ name: 'change', id: it.changeId })
    else api.openURL(it.url)
  }
</script>

<div class="page">
  <header style="--wails-draggable: drag">
    <div>
      <div class="muted mono small">Longest-waiting reviews first</div>
      <h1>Inbox</h1>
    </div>
    <button class="btn" style="--wails-draggable: no-drag" onclick={() => { loadInbox(true); loadCleanPlan() }} disabled={app.inboxLoading}>
      <Icon name="refresh" spin={app.inboxLoading} />Refresh
    </button>
  </header>

  <section>
    <div class="eyebrow warnlabel">Waiting on your review · {review.length}</div>
    {#if app.inbox?.reviewError}<div class="row muted">GitHub: {app.inbox.reviewError}</div>{/if}
    {#each review as it (it.url)}
      <div class="row">
        <div class="repo mono">{it.repo}#{it.number}</div>
        <div class="stack">
          <span class="title">{it.title}</span>
          <span class="muted small">by {it.author}{it.draft ? ' · draft' : ''}</span>
        </div>
        <div class="age mono muted">{ago(it.updatedAt)}</div>
        <button class="btn primary small" onclick={() => api.openURL(it.url)}>Review <Icon name="external" size={14} /></button>
      </div>
    {:else}
      {#if app.inbox && !app.inbox.reviewError}<div class="row empty muted">Nobody is waiting on you.</div>{/if}
    {/each}
  </section>

  {#if needAction.length || ready.length}
    <section>
      <div class="eyebrow">Your changes</div>
      {#each [...needAction, ...ready] as c (c.id)}
        <button class="row link" onclick={() => navigate({ name: 'change', id: c.id })}>
          <div class="repo mono accent">{c.id}</div>
          <div class="stack">
            <span class="title">{c.title || 'Untitled change'}</span>
            <span class="small {c.tone}">{c.headline}</span>
          </div>
          <div class="age"></div>
          <span class="muted small">Open →</span>
        </button>
      {/each}
    </section>
  {/if}

  <section>
    <div class="eyebrow">Your open PRs · {mine.length}</div>
    {#if app.inbox?.mineError}<div class="row muted">GitHub: {app.inbox.mineError}</div>{/if}
    {#each mine as it (it.url)}
      <button class="row link" onclick={() => open(it)}>
        <div class="repo mono">{it.repo}#{it.number}</div>
        <div class="stack">
          <span class="title">{it.title}</span>
          <span class="muted small">{it.draft ? 'draft · ' : ''}{it.changeId ? `part of ${it.changeId}` : 'not tracked by Tandem'}</span>
        </div>
        <div class="age mono muted">{ago(it.updatedAt)}</div>
        <span class="muted small">{it.changeId ? 'Open change →' : 'GitHub ↗'}</span>
      </button>
    {/each}
  </section>
</div>

{#if cleanable.length}
  <div class="page housekeeping">
    <section>
      <div class="eyebrow">Housekeeping</div>
      <div class="row dashed">
        <div class="repo mono">{cleanable.length} landed change{cleanable.length === 1 ? '' : 's'}</div>
        <div class="stack">
          <span class="title">{cleanable.reduce((n, c) => n + c.worktrees.length, 0)} worktrees and their branches can go</span>
          <span class="muted small">{cleanable.map((c) => c.id).join(', ')} · every path is listed before anything is removed</span>
        </div>
        <div class="age"></div>
        <button class="btn small" onclick={() => (cleaning = true)}>Review cleanup</button>
      </div>
    </section>
  </div>
{/if}

{#if cleaning}
  <CleanDialog items={cleanable} onclose={() => (cleaning = false)} />
{/if}

<style>
  .page { padding: 0 32px 32px; display: flex; flex-direction: column; gap: 26px; }
  header { display: flex; justify-content: space-between; align-items: flex-end; padding-top: 28px; }
  h1 { margin: 4px 0 0; font-family: var(--display); font-weight: 700; font-size: 30px; }
  .small { font-size: 12px; }
  section { display: flex; flex-direction: column; gap: 8px; }
  .warnlabel { color: var(--warn-text); }
  .row {
    display: grid; grid-template-columns: 260px minmax(0, 1fr) 56px auto; gap: 18px; align-items: center;
    padding: 13px 18px; background: var(--panel); border: 1px solid var(--line); border-radius: 12px;
    text-align: left; color: var(--text);
  }
  .row.empty { display: block; background: transparent; border-style: dashed; }
  .row.dashed { background: transparent; border-style: dashed; }
  .housekeeping { padding-top: 0; }
  .row.link { cursor: pointer; }
  .row.link:hover { background: var(--raised); }
  .repo { font-size: 12.5px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--text-2); }
  .accent { color: var(--accent-text); }
  .stack { display: flex; flex-direction: column; gap: 3px; min-width: 0; }
  .title { font-size: 14.5px; font-weight: 500; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .age { font-size: 12px; text-align: right; }
</style>
