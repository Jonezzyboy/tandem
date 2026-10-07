<script lang="ts">
  import { onDestroy, onMount } from 'svelte'
  import { api, errorText } from '@lib/api'
  import { navigate } from '@lib/state.svelte'
  import { finish, loadStatus, pullLatest, startTest, tester, verdict } from '@lib/tester.svelte'
  import type { PlanRepo, TesterPlan, TestPR, TestStep, VerdictOption } from '@lib/types'
  import Icon from './Icon.svelte'

  let { id }: { id: string } = $props()

  let plan = $state<TesterPlan | null>(null)
  let planError = $state('')
  async function load() {
    planError = ''
    try {
      plan = await api.testerPlan(id)
    } catch (e) {
      planError = errorText(e)
    }
  }
  onMount(load)

  const session = $derived(tester.state.current?.key === id ? tester.state.current : null)
  const other = $derived(tester.state.current && tester.state.current.key !== id ? tester.state.current : null)
  const starting = $derived(tester.starting === id)
  // A start that left some repos behind stays on screen until dismissed.
  let lastSteps = $state<TestStep[] | null>(null)

  const usable = $derived(plan ? plan.repos.filter((r) => r.cloned || plan!.cloneRoot) : [])
  const uncloneable = $derived(plan ? plan.repos.filter((r) => !r.cloned && !plan!.cloneRoot) : [])
  const dirty = $derived(usable.filter((r) => r.cloned && r.dirty > 0 && r.current !== r.branch))
  let setAside = $state(true)

  const title = $derived(plan?.ticket.summary || session?.title || '')
  const url = $derived(plan?.ticket.url || session?.url || '')

  async function test() {
    if (!plan) return
    lastSteps = null
    const steps = await startTest(id, title, url, usable.map((r) => ({ name: r.name, branch: r.branch })), setAside && dirty.length > 0)
    if (steps.some((s) => !s.ok)) lastSteps = steps
    load()
  }

  // While testing, GitHub is checked for new commits every minute.
  const tick = setInterval(() => { if (session) loadStatus() }, 60_000)
  onDestroy(() => clearInterval(tick))
  $effect(() => {
    if (session) loadStatus()
  })

  const status = $derived(Object.fromEntries(tester.status.map((s) => [s.name, s])))
  const behind = $derived(tester.status.filter((s) => s.behind > 0))

  async function backToMain(verdictMsg = '') {
    if (await finish(verdictMsg)) navigate({ name: 'ready' })
  }

  // The ticket's own transitions, offered as the test result.
  let verdicts = $state<VerdictOption[] | null>(null)
  let verdictsError = $state('')
  $effect(() => {
    if (!session || verdicts) return
    api.testerVerdicts(id).then((v) => (verdicts = v)).catch((e) => (verdictsError = errorText(e)))
  })
  let choice = $state<VerdictOption | null>(null)
  let note = $state('')
  let thenBack = $state(true)
  let recording = $state(false)
  let recorded = $state('')
  const noteLabel = $derived(choice?.outcome === 'failed' ? 'What went wrong?' : choice?.outcome === 'passed' ? 'What did you check? (optional)' : 'Note (optional)')
  const canRecord = $derived(!!choice && (choice.outcome !== 'failed' || note.trim() !== ''))
  async function record() {
    if (!choice || !canRecord) return
    recording = true
    const msg = await verdict(choice.id, note)
    recording = false
    if (!msg) return
    if (thenBack) await backToMain(msg)
    else recorded = msg
  }

  function prText(pr: TestPR | null): { text: string; tone: string } {
    if (!pr) return { text: 'no open PR', tone: 'muted' }
    if (pr.fail) return { text: `${pr.fail} failing`, tone: 'warn' }
    if (pr.pending) return { text: 'checks running', tone: 'muted' }
    if (pr.pass) return { text: 'passing', tone: 'ok' }
    return { text: 'no checks', tone: 'muted' }
  }

  function machine(r: PlanRepo): { text: string; tone: string } {
    if (!r.cloned) return plan?.cloneRoot ? { text: 'not cloned yet: Tandem clones it', tone: 'warn' } : { text: 'not cloned, and cloning is off in Settings', tone: 'warn' }
    const parts = [r.current ? `on ${r.current}` : 'detached HEAD']
    if (r.dirty) parts.push(`${r.dirty} uncommitted file${r.dirty === 1 ? '' : 's'}`)
    else parts.push('clean')
    return { text: parts.join(' · '), tone: r.dirty && r.current !== r.branch ? 'warn' : 'muted' }
  }

  const progress = $derived(lastSteps ?? Object.values(tester.steps))
  const doneCount = $derived(progress.filter((s) => s.done).length)
  const since = $derived(session ? new Date(session.started).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }) : '')
</script>

<div class="page">
  <header style="--wails-draggable: drag">
    <div class="heading">
      <div class="meta mono">
        {#if url}<button class="link" style="--wails-draggable: no-drag" onclick={() => api.openURL(url)}>{id}<Icon name="external" size={11} /></button>{:else}<span>{id}</span>{/if}
        {#if session}
          <span>·</span><span class="ok-text">testing since {since}</span><span>·</span><span>{session.repos.length} repo{session.repos.length === 1 ? '' : 's'} on {id}</span>
        {:else if plan?.ticket.status}
          <span>·</span><span class="ok-text">{plan.ticket.status}</span>
          {#if plan.ticket.assignee}<span>·</span><span>{plan.ticket.assignee}</span>{/if}
        {/if}
      </div>
      <h1>{starting || lastSteps ? `Switching to ${id}` : title || id}</h1>
    </div>
    <div class="actions" style="--wails-draggable: no-drag">
      {#if session && !starting && !lastSteps}
        <button class="btn" disabled={tester.finishing} onclick={() => backToMain(recorded)}>
          <Icon name="refresh" spin={tester.finishing} />Back to main
        </button>
      {:else if !session && !starting && !lastSteps}
        <button class="btn primary big" disabled={!plan || usable.length === 0 || !!other} onclick={test}>
          <Icon name="branch" />Test this change
        </button>
      {/if}
    </div>
  </header>

  {#if planError}
    <div class="note warn selectable"><Icon name="alert" color="var(--warn)" />{planError}</div>
  {/if}

  {#if starting || lastSteps}
    <section class="progress" aria-label="Progress">
      <div class="progress-head">
        <span class="eyebrow">{doneCount} of {progress.length} repos {starting ? 'done' : 'finished'}</span>
        <div class="bar"><div style:width="{progress.length ? (doneCount / progress.length) * 100 : 0}%"></div></div>
      </div>
      {#each progress as s (s.repo)}
        <div class="step">
          {#if !s.done}<Icon name="running" spin color="var(--muted)" />
          {:else if s.ok}<Icon name="check" color="var(--ok)" />
          {:else}<Icon name="x" color="var(--warn)" />{/if}
          <div class="stack">
            <span class="mono">{s.repo}</span>
            <span class="small" class:muted={s.ok || !s.done} class:warn={s.done && !s.ok}>{s.message}</span>
          </div>
          {#if s.sha}<span class="mono small muted">at {s.sha}</span>{/if}
        </div>
      {/each}
    </section>
    <section class="panel narrow">
      <span class="eyebrow">Before you start</span>
      <span class="small sub">Restart anything running from these repos (local services, dev servers) so it picks up the change's code.</span>
    </section>
    {#if lastSteps}
      <div class="row-actions">
        {#if session}<button class="btn primary" onclick={() => (lastSteps = null)}>Test with the repos that switched</button>{/if}
        <button class="btn" disabled={tester.finishing} onclick={() => { lastSteps = null; backToMain() }}>Back to main</button>
      </div>
    {/if}
  {:else if session}
    {#if behind.length}
      <div class="banner">
        <Icon name="sync" />
        <span class="grow">
          {#each behind as b, i (b.name)}
            {i ? ' · ' : ''}{b.behind} new commit{b.behind === 1 ? '' : 's'} in <span class="mono">{b.name.split('/')[1]}</span>{#if b.new.length}: <span class="sub">{b.new.map((m) => `"${m}"`).join(', ')}</span>{/if}
          {/each}
        </span>
        <button class="btn small primary" disabled={tester.pulling} onclick={pullLatest}>
          <Icon name="sync" size={14} spin={tester.pulling} />Pull latest
        </button>
      </div>
    {/if}
    <div class="cols">
      <section class="repos" aria-label="Repos">
        <div class="row head eyebrow"><div>Repo</div><div>PR</div><div>On your machine</div></div>
        {#each session.repos as r (r.name)}
          {@const st = status[r.name]}
          {@const pr = plan?.repos.find((p) => p.name === r.name)?.pr ?? null}
          {@const ci = prText(pr)}
          <div class="row" class:warnrow={st && (!st.onBranch || st.behind > 0 || st.error)}>
            <span class="mono">{r.name}</span>
            <div>{#if pr}<button class="link mono" onclick={() => api.openURL(pr.url)}>#{pr.number}</button> {/if}<span class={ci.tone}>{pr ? '· ' : ''}{ci.text}</span></div>
            {#if !st}
              <span class="muted">{tester.statusLoading ? 'checking…' : '—'}</span>
            {:else if st.error}
              <span class="warn">{st.error}</span>
            {:else if !st.onBranch}
              <span class="warn">on {st.current || 'a detached HEAD'}, not {st.branch}</span>
            {:else if st.behind}
              <span class="warn">on {st.branch} · {st.behind} commit{st.behind === 1 ? '' : 's'} behind</span>
            {:else}
              <span class="ok-text">on {st.branch} · up to date{st.dirty ? ` · ${st.dirty} edited` : ''}</span>
            {/if}
          </div>
        {/each}
      </section>
      <section class="panel" aria-label="Your verdict">
        <span class="eyebrow">Your verdict</span>
        {#if recorded}
          <div class="recorded"><Icon name="check" color="var(--ok)" /><span>{recorded}.</span></div>
          <button class="btn primary" disabled={tester.finishing} onclick={() => backToMain(recorded)}>
            <Icon name="refresh" spin={tester.finishing} />Back to main
          </button>
        {:else if verdictsError}
          <p class="small warn">Couldn't read the ticket's workflow: {verdictsError}</p>
          <p class="small muted">Record the result in Jira, then go back to main.</p>
        {:else if !verdicts}
          <p class="small muted">Reading {id}'s workflow…</p>
        {:else if verdicts.length === 0}
          <p class="small muted">{id} has no moves from its current status. Record the result in Jira, then go back to main.</p>
        {:else}
          <fieldset class="choices">
            <legend class="small sub">How did {id} go? Each choice is a move in its Jira workflow.</legend>
            {#each verdicts as v (v.id)}
              <label class="choice {v.outcome}" class:on={choice?.id === v.id}>
                <input type="radio" name="verdict" checked={choice?.id === v.id} onchange={() => (choice = v)} />
                <Icon name={v.outcome === 'passed' ? 'check' : v.outcome === 'failed' ? 'x' : 'send'} />
                <span class="stack"><span class="choice-name">{v.name}</span><span class="small muted">moves it to {v.to}</span></span>
              </label>
            {/each}
          </fieldset>
          {#if choice}
            <label class="small sub" for="t-note">{noteLabel}</label>
            <textarea id="t-note" class="input" rows="3" bind:value={note}
              placeholder={choice.outcome === 'failed' ? 'Steps to reproduce, what you expected, what happened' : 'Browsers, pages or cases you covered'}></textarea>
            <label class="then">
              <input type="checkbox" bind:checked={thenBack} />
              Then put every repo back on main
            </label>
            <button class="btn record {choice.outcome}" disabled={!canRecord || recording || tester.finishing} onclick={record}>
              <Icon name={recording || tester.finishing ? 'running' : choice.outcome === 'failed' ? 'x' : 'check'} spin={recording || tester.finishing} />
              {choice.name}: move to {choice.to}{thenBack ? ' and go back' : ''}
            </button>
            <span class="small muted">{note.trim() ? 'Your note is added to the ticket as a comment.' : choice.outcome === 'failed' ? 'Add what went wrong so the developer can fix it.' : 'No comment is added without a note.'}</span>
          {/if}
        {/if}
      </section>
    </div>
  {:else}
    {#if other}
      <div class="note"><Icon name="alert" color="var(--warn)" /><span class="grow">You're testing <span class="mono">{other.key}</span>. Go back to main before testing another change.</span>
        <button class="btn small" onclick={() => navigate({ name: 'test', id: other.key })}>Open {other.key}</button></div>
    {:else if plan && usable.length}
      <div class="note"><Icon name="alert" color="var(--accent-text)" />
        <span>Testing switches {usable.length === 1 ? 'the repo' : `the ${usable.length} repos`} below to {id}'s branch{usable.length === 1 ? '' : 'es'} with the latest pushed commits. One click puts {usable.length === 1 ? 'it' : 'them all'} back on main when you're done.</span>
      </div>
    {:else if plan}
      <div class="note"><Icon name="alert" color="var(--warn)" /><span>No open PR names {id} in its branch or title, and none of your clones has a <span class="mono">{id}</span> branch on origin. If it's already merged, test it on main.</span></div>
    {/if}
    {#if plan?.error}<div class="small warn">{plan.error}</div>{/if}
    <div class="cols">
      <section class="repos" aria-label="Repos">
        <div class="row head eyebrow"><div>Repo</div><div>PR</div><div>On your machine</div></div>
        {#if !plan}
          <div class="row"><span class="muted">Looking for {id} on GitHub and in your clones…</span></div>
        {/if}
        {#each plan?.repos ?? [] as r (r.name)}
          {@const ci = prText(r.pr)}
          {@const m = machine(r)}
          <div class="row" class:warnrow={!r.cloned}>
            <span class="stack"><span class="mono">{r.name}</span>{#if r.branch && r.branch !== id}<span class="mono small muted">{r.branch}</span>{/if}</span>
            <div>{#if r.pr}<button class="link mono" onclick={() => api.openURL(r.pr!.url)}>#{r.pr.number}</button> {/if}<span class={ci.tone}>{r.pr ? '· ' : ''}{ci.text}</span></div>
            <span class={m.tone}>{m.text}</span>
          </div>
        {/each}
        {#if dirty.length}
          <label class="aside">
            <input type="checkbox" bind:checked={setAside} />
            <span class="stack">
              <span>Set aside my uncommitted work in {dirty.map((r) => r.name.split('/')[1]).join(', ')}</span>
              <span class="small muted">Stashed while you test, and put back when you return to main. Unticked, those repos stay where they are.</span>
            </span>
          </label>
        {/if}
        {#if uncloneable.length}
          <div class="aside small muted">{uncloneable.length} repo{uncloneable.length === 1 ? '' : 's'} will be skipped: turn on cloning in Settings to include {uncloneable.length === 1 ? 'it' : 'them'}.</div>
        {/if}
      </section>
      <section class="panel" aria-label="What to test">
        <div class="panel-head">
          <span class="eyebrow">What to test · from Jira</span>
          {#if plan?.ticket.priority || plan?.ticket.type}<span class="mono small muted">{[plan.ticket.priority, plan.ticket.type].filter(Boolean).join(' · ')}</span>{/if}
        </div>
        {#if !plan}
          <p class="small muted">Reading the ticket…</p>
        {:else if plan.ticket.error}
          <p class="small warn">{plan.ticket.error}</p>
        {:else if plan.ticket.description}
          <div class="desc selectable">{plan.ticket.description}</div>
        {:else}
          <p class="small muted">The ticket has no description.</p>
        {/if}
        {#if url}<button class="link small" onclick={() => api.openURL(url)}>Open the full ticket in Jira <Icon name="external" size={12} /></button>{/if}
      </section>
    </div>
  {/if}
</div>

<style>
  .page { padding: 0 32px 32px; display: flex; flex-direction: column; gap: 18px; }
  header { padding-top: 28px; display: flex; justify-content: space-between; align-items: flex-end; gap: 24px; }
  .heading { display: flex; flex-direction: column; gap: 6px; min-width: 0; }
  .meta { display: flex; gap: 8px; font-size: 12px; color: var(--muted); align-items: center; flex-wrap: wrap; }
  h1 { margin: 0; font-family: var(--display); font-weight: 700; font-size: 30px; line-height: 1.15; }
  .actions { display: flex; gap: 8px; flex-shrink: 0; }
  .big { min-height: 42px; padding: 0 18px; }
  .small { font-size: 12px; }
  .grow { flex: 1; min-width: 0; }
  .stack { display: flex; flex-direction: column; gap: 2px; min-width: 0; }
  .sub { color: var(--text-2); }
  .ok, .ok-text { color: var(--ok-text); }
  .link { display: inline-flex; align-items: center; gap: 4px; border: 0; background: none; padding: 0; color: var(--accent-text); cursor: pointer; }
  .link:hover { color: var(--link-hover); }
  .note { display: flex; gap: 12px; align-items: center; padding: 10px 12px 10px 14px; border-radius: 10px; background: var(--panel); border: 1px solid var(--line); font-size: 13px; color: var(--text-2); }
  .banner {
    display: flex; gap: 12px; align-items: center; padding: 10px 12px 10px 16px; border-radius: 10px;
    background: var(--warn-row); border: 1px solid var(--warn-border); color: var(--warn-text); font-size: 13px;
  }
  .cols { display: grid; grid-template-columns: minmax(0, 1fr) 400px; gap: 16px; align-items: start; }
  .repos { border: 1px solid var(--line); border-radius: 12px; overflow: hidden; }
  .row { display: grid; grid-template-columns: minmax(0, 1.5fr) minmax(0, 1fr) minmax(0, 1.5fr); gap: 14px; padding: 12px 16px; align-items: center; border-top: 1px solid var(--line); font-size: 13px; }
  .row > * { min-width: 0; overflow: hidden; text-overflow: ellipsis; }
  .row.head { border-top: 0; background: var(--panel); padding: 10px 16px; }
  .row.warnrow { background: var(--warn-row); }
  .aside { display: flex; gap: 10px; align-items: flex-start; padding: 14px 16px; border-top: 1px solid var(--line); background: var(--panel); font-size: 13px; }
  .aside input { width: 16px; height: 16px; margin: 2px 0 0; accent-color: var(--accent); }
  .panel { display: flex; flex-direction: column; gap: 10px; padding: 16px 18px; background: var(--panel); border: 1px solid var(--line); border-radius: 12px; }
  .panel p { margin: 0; }
  .panel.narrow { max-width: 760px; }
  .panel-head { display: flex; justify-content: space-between; align-items: center; gap: 12px; }
  .desc { white-space: pre-wrap; line-height: 1.55; color: var(--text-2); max-height: 360px; overflow: auto; font-size: 13.5px; }
  textarea.input { width: 100%; font-size: 13px; }
  .choices { border: 0; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 8px; }
  .choices legend { margin-bottom: 8px; padding: 0; }
  .choice {
    display: grid; grid-template-columns: 16px 16px minmax(0, 1fr); gap: 10px; align-items: center; padding: 10px 12px;
    border-radius: 10px; border: 1px solid var(--line-2); background: var(--raised); cursor: pointer;
  }
  .choice input { margin: 0; accent-color: var(--accent); }
  .choice-name { font-weight: 500; font-size: 14px; }
  .choice.passed :global(svg) { color: var(--ok); }
  .choice.failed :global(svg) { color: var(--warn); }
  .choice.on.passed { border-color: var(--ok-border); background: var(--ok-bg); }
  .choice.on.failed { border-color: var(--warn-border); background: var(--warn-bg); }
  .choice.on.moved { border-color: var(--accent); background: var(--accent-bg); }
  .then { display: flex; gap: 8px; align-items: center; font-size: 13px; cursor: pointer; }
  .then input { width: 16px; height: 16px; margin: 0; accent-color: var(--accent); }
  .record { justify-content: center; min-height: 40px; white-space: normal; text-align: center; }
  .record.passed { border-color: var(--ok-border); background: var(--ok-bg); color: var(--ok-text); }
  .record.failed { border-color: var(--warn-border); background: var(--warn-bg); color: var(--warn-text); }
  .record.moved { border-color: var(--accent); background: var(--accent); color: var(--on-accent); }
  .recorded { display: flex; gap: 10px; align-items: center; font-size: 14px; }
  .progress { max-width: 760px; border: 1px solid var(--line); border-radius: 12px; overflow: hidden; }
  .progress-head { display: flex; justify-content: space-between; align-items: center; padding: 12px 16px; background: var(--panel); }
  .bar { width: 220px; height: 6px; border-radius: 3px; background: var(--line); overflow: hidden; }
  .bar > div { height: 100%; background: var(--ok); transition: width 0.3s ease-out; }
  .step { display: grid; grid-template-columns: 18px minmax(0, 1fr) auto; gap: 12px; align-items: center; padding: 14px 16px; border-top: 1px solid var(--line); font-size: 13px; }
  .row-actions { display: flex; gap: 8px; }
</style>
