<script lang="ts">
  import { api } from '@lib/api'
  import { fail, navigate } from '@lib/state.svelte'
  import { prefs } from '@lib/settings.svelte'
  import { parseTicket } from '@lib/jira'
  import type { JiraTicket, StartItem } from '@lib/types'
  import ExistingBranches from './ExistingBranches.svelte'
  import Icon from './Icon.svelte'
  import RepoPicker from './RepoPicker.svelte'

  let id = $state('')
  let title = $state('')
  let selected = $state<string[]>([])
  let existing = $state<string[]>([])
  let worktrees = $state(false)
  let starting = $state(false)
  let results = $state<StartItem[] | null>(null)

  let ticket = $state<JiraTicket | null>(null)
  let reading = $state(false)
  // The title as last filled from Jira, so a later lookup only replaces it while it's untouched.
  let jiraTitle = $state('')
  let titleInput = $state<HTMLInputElement>()
  let lookups = 0

  function onIdInput() {
    const t = parseTicket(id)
    if (t) {
      id = t.key
      readTicket(t)
    } else if (ticket && id.trim().toUpperCase() !== ticket.key) {
      ticket = null
    }
  }

  async function readTicket(t: { key: string; url: string }) {
    const n = ++lookups
    ticket = { ...t, summary: '', type: '', status: '', error: '', noAccess: false }
    reading = true
    try {
      const got = await api.jiraLookup(t.url)
      if (n !== lookups) return
      ticket = got
      if (got.summary && (!title.trim() || title === jiraTitle)) title = jiraTitle = got.summary
      if (got.error && !title.trim()) titleInput?.focus()
    } catch (e) {
      if (n === lookups) fail(e)
    } finally {
      if (n === lookups) reading = false
    }
  }

  function unlink() {
    lookups++
    ticket = null
    reading = false
  }

  const validId = $derived(/^[A-Za-z0-9][A-Za-z0-9._-]*$/.test(id) && !id.includes('..'))
  // ExistingBranches unmounts on an invalid ID and leaves its last matches behind.
  const loadable = $derived(validId ? existing : [])
  const loading = $derived(selected.filter((s) => loadable.includes(s)).length)
  const creating = $derived(selected.length - loading)

  const plural = (n: number, word: string) => `${n} ${word}${n === 1 ? '' : word.endsWith('h') ? 'es' : 's'}`
  const action = $derived.by(() => {
    const noun = worktrees ? 'worktree' : 'branch'
    if (!selected.length) return `Create ${noun}${worktrees ? 's' : 'es'}`
    if (!loading) return `Create ${plural(creating, noun)}`
    if (!creating) return `Load ${plural(loading, 'existing branch')}${worktrees ? ' into worktrees' : ''}`
    if (worktrees) return `Create ${plural(selected.length, noun)}, ${loading} from existing branches`
    return `Load ${plural(loading, 'existing branch')}, create ${creating} new`
  })

  function unpick(name: string) {
    selected = selected.filter((s) => s !== name)
  }

  async function start() {
    starting = true
    try {
      results = await api.start(id, title, selected, worktrees, ticket?.url ?? '')
      if (results.every((r) => r.ok)) navigate({ name: 'change', id })
    } catch (e) {
      fail(e)
    } finally {
      starting = false
    }
  }
</script>

<div class="page">
  <header style="--wails-draggable: drag">
    <div class="muted mono small">{worktrees ? 'One worktree per repo, beside your clones' : 'One branch per repo, checked out in your clones'}</div>
    <h1>New change</h1>
  </header>

  <div class="grid">
    <div class="form">
      <div class="field">
        <label for="nc-id">Jira ticket or change ID</label>
        <div class="id-wrap" class:linked={ticket}>
          <Icon name="link" />
          <input id="nc-id" class="input mono" bind:value={id} oninput={onIdInput} placeholder="Paste a Jira link, or type DEV-123" autocomplete="off" />
        </div>
        {#if ticket}
          <div class="ticket" class:plain={reading || ticket.error}>
            {#if reading}
              <Icon name="running" spin color="var(--muted)" />
              <span class="ticket-body">Reading {ticket.key} from Jira…</span>
            {:else if ticket.error}
              <Icon name="alert" color="var(--warn)" />
              <div class="ticket-body">
                <div class="strong">Linked to {ticket.key}, but Tandem can’t read it</div>
                <div class="small sub">{ticket.noAccess ? 'Jira asked for a sign-in' : ticket.error}, so the title is up to you. The link is still kept.</div>
                <div class="ticket-links small">
                  {#if ticket.noAccess}<button class="link" onclick={() => navigate({ name: 'settings' })}>Connect Jira in Settings</button>{/if}
                  <button class="link" onclick={() => api.openURL(ticket!.url)}>Open in Jira <Icon name="external" size={12} /></button>
                </div>
              </div>
            {:else}
              <div class="ticket-body">
                <div class="mono small ticket-meta">
                  <span class="accent">{ticket.key}</span>
                  {#if ticket.type}<span class="muted">·</span><span>{ticket.type}</span>{/if}
                  {#if ticket.status}<span class="muted">·</span><span>{ticket.status}</span>{/if}
                </div>
                <div class="strong">{ticket.summary}</div>
                <button class="link small" onclick={() => api.openURL(ticket!.url)}>{ticket.url.replace('https://', '')} <Icon name="external" size={12} /></button>
              </div>
            {/if}
            <button class="icon-btn unlink" aria-label="Unlink from Jira, keep the ID" title="Unlink from Jira" onclick={unlink}><Icon name="close" size={14} /></button>
          </div>
        {/if}
        <span class="small muted">{ticket ? '' : 'A Jira link fills in the ID and title and keeps the link. '}{worktrees ? 'The branch name in every repo’s worktree.' : 'The branch name in every repo. Clean repos switch to it; any with uncommitted work stay put.'} Repos that already have it keep their commits.</span>
      </div>
      {#if validId}<ExistingBranches branch={id} bind:selected bind:existing />{/if}
      <div class="field">
        <div class="label-row">
          <label for="nc-title">Title</label>
          {#if jiraTitle && title === jiraTitle}<span class="mono muted tiny">from Jira</span>{/if}
        </div>
        <input id="nc-title" class="input" bind:this={titleInput} bind:value={title} placeholder="What this change does" />
      </div>
      <div class="field">
        <span class="label">Repos · {selected.length} selected</span>
        <div class="chips">
          {#each selected as s (s)}
            {@const old = loadable.includes(s)}
            <button class="chip mono" class:existing={old} onclick={() => unpick(s)} aria-label="Remove {s}" title={old ? `Loads the existing ${id} branch` : undefined}>
              {#if old}<Icon name="branch" size={12} />{/if}{s} <Icon name="close" size={12} />
            </button>
          {:else}
            <span class="small muted">Pick repos from the list →</span>
          {/each}
        </div>
      </div>
      <label class="mode">
        <input type="checkbox" bind:checked={worktrees} />
        <span class="stack">
          <span>Use worktrees instead</span>
          <span class="small muted">A worktree per repo under <span class="mono">{prefs.home || '~/code/.tandem'}/{id || 'ID'}</span>, so your clones and anything running from them stay on their current branches. Repos added later follow this.</span>
        </span>
      </label>
      <button class="btn primary start" disabled={!validId || selected.length === 0 || starting} onclick={start}>
        <Icon name="branch" spin={starting} />
        {action}
      </button>
      {#if results}
        <div class="results">
          {#each results as r (r.repo)}
            <div class="result">
              {#if r.ok}<Icon name="check" color="var(--ok)" />{:else}<Icon name="x" color="var(--warn)" />{/if}
              <span class="mono">{r.repo}</span>
              <span class="small selectable" class:warn={!r.ok} class:muted={r.ok}>{r.message}</span>
            </div>
          {/each}
          {#if results.some((r) => r.ok)}
            <button class="btn small" onclick={() => navigate({ name: 'change', id })}>Open {id} →</button>
          {/if}
        </div>
      {/if}
    </div>

    <RepoPicker bind:selected />
  </div>
</div>

<style>
  .page { padding: 0 32px 32px; display: flex; flex-direction: column; gap: 24px; height: 100%; }
  header { padding-top: 28px; }
  h1 { margin: 4px 0 0; font-family: var(--display); font-weight: 700; font-size: 30px; }
  .small { font-size: 12px; }
  .grid { display: grid; grid-template-columns: 420px minmax(0, 1fr); gap: 20px; flex: 1; min-height: 0; }
  .form { display: flex; flex-direction: column; gap: 18px; padding: 20px; background: var(--panel); border: 1px solid var(--line); border-radius: 12px; align-self: start; }
  .label { font-size: 13px; color: var(--text-2); }
  .chips { display: flex; flex-wrap: wrap; gap: 6px; }
  .chip { display: inline-flex; align-items: center; gap: 6px; padding: 5px 9px; border-radius: 7px; border: 1px solid var(--line-2); background: var(--raised); font-size: 12.5px; cursor: pointer; }
  .chip.existing { border-color: var(--accent); background: var(--accent-bg); color: var(--accent-text); }
  .id-wrap { position: relative; display: flex; }
  .id-wrap .input { flex: 1; padding-left: 36px; }
  .id-wrap :global(svg) { position: absolute; left: 12px; top: 50%; transform: translateY(-50%); color: var(--muted); pointer-events: none; }
  .id-wrap.linked :global(svg) { color: var(--accent-text); }
  .ticket {
    display: flex; gap: 12px; align-items: flex-start; padding: 12px 8px 12px 14px; border-radius: 10px;
    background: var(--accent-bg); border: 1px solid var(--accent); font-size: 14px;
  }
  .ticket.plain { background: var(--raised); border-color: var(--line-2); }
  .ticket > :global(svg) { margin-top: 2px; }
  .ticket-body { display: flex; flex-direction: column; gap: 3px; flex: 1; min-width: 0; }
  .ticket-meta { display: flex; gap: 8px; color: var(--text-2); }
  .ticket-meta .accent { color: var(--accent-text); }
  .strong { font-weight: 500; }
  .sub { color: var(--text-2); line-height: 1.45; }
  .ticket-links { display: flex; gap: 14px; margin-top: 2px; }
  .link { display: inline-flex; align-items: center; gap: 5px; border: 0; background: none; padding: 0; color: var(--accent-text); cursor: pointer; text-align: left; }
  .link:hover { color: var(--link-hover); }
  .ticket-body > .link { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .unlink { flex-shrink: 0; width: 28px; height: 28px; color: var(--accent-text); }
  .ticket.plain .unlink { color: var(--muted); }
  .label-row { display: flex; justify-content: space-between; align-items: baseline; }
  .tiny { font-size: 11px; }
  .start { justify-content: center; min-height: 42px; }
  .mode { display: flex; gap: 10px; align-items: flex-start; cursor: pointer; font-size: 14px; }
  .mode input { width: 16px; height: 16px; margin-top: 2px; accent-color: var(--accent); }
  .mode .stack { display: flex; flex-direction: column; gap: 3px; line-height: 1.4; }
  .mode .mono { white-space: nowrap; }
  .results { display: flex; flex-direction: column; gap: 8px; }
  .result { display: grid; grid-template-columns: 16px auto; gap: 4px 10px; align-items: center; font-size: 13px; }
  .result span:last-child { grid-column: 2; word-break: break-all; }
</style>
