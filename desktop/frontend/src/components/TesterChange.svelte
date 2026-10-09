<script lang="ts">
  import { onDestroy, onMount } from 'svelte'
  import { api, errorText } from '@lib/api'
  import { navigate } from '@lib/state.svelte'
  import { buildKey, finish, loadBuilds, loadStatus, pullLatest, runBuild, saveChecks, startTest, stopBuild, tester, verdict } from '@lib/tester.svelte'
  import { ago, shortRepo } from '@lib/format'
  import type { BuildTarget, PlanRepo, TestCheck, TesterPlan, TestPR, TestStep, VerdictOption } from '@lib/types'
  import Icon from './Icon.svelte'

  let { id }: { id: string } = $props()

  let plan = $state<TesterPlan | null>(null)
  let planError = $state('')
  // The checklist to start with, edited before testing begins.
  let draft = $state<TestCheck[] | null>(null)
  async function load() {
    planError = ''
    try {
      plan = await api.testerPlan(id)
      draft ??= [...plan.checklist]
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
    const steps = await startTest(id, title, url, usable.map((r) => ({ name: r.name, branch: r.branch })), setAside && dirty.length > 0, draft ?? [])
    if (steps.some((s) => !s.ok)) lastSteps = steps
    load()
  }

  // While testing, origin is checked for new commits every minute.
  const tick = setInterval(() => { if (session) loadStatus() }, 60_000)
  onDestroy(() => clearInterval(tick))
  $effect(() => {
    if (session) {
      loadStatus()
      loadBuilds()
    }
  })

  // The build whose output is open, by buildKey.
  let openLog = $state('')
  function buildState(t: BuildTarget): { text: string; tone: string } {
    const b = tester.builds[buildKey(t)]
    if (!b) return { text: 'not built since switching', tone: 'muted' }
    if (b.running) return { text: 'building…', tone: 'muted' }
    if (!b.ok) return { text: b.error === 'stopped' ? 'stopped' : `failed${b.error ? `: ${b.error}` : ''}`, tone: 'warn' }
    if (b.at < (tester.movedAt[t.repo] ?? 0)) return { text: 'built before the latest pull: rebuild', tone: 'warn' }
    return { text: `built ${ago(b.at, now)} ago`, tone: 'ok' }
  }
  const anyBuilding = $derived(tester.buildTargets.some((t) => tester.builds[buildKey(t)]?.running))
  function buildAll() {
    for (const t of tester.buildTargets) if (!tester.builds[buildKey(t)]?.running) runBuild(t)
  }
  function scrollEnd(node: HTMLElement, _lines: number) {
    node.scrollTop = node.scrollHeight
    return { update() { node.scrollTop = node.scrollHeight } }
  }

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
  const checks = $derived(session?.checks ?? [])
  const checked = $derived(checks.filter((c) => c.done).length)
  function toggleCheck(i: number) {
    saveChecks(checks.map((c, j) => (j === i ? { ...c, done: !c.done } : c)))
  }
  let newCheck = $state('')
  function addCheck() {
    const text = newCheck.trim()
    if (!text) return
    if (session) saveChecks([...checks, { text, done: false }])
    else draft = [...(draft ?? []), { text, done: false }]
    newCheck = ''
  }

  // sections splits a checklist into runs of one group, keeping each check's
  // index in the whole list.
  function sections(list: TestCheck[]): { group: string; items: { c: TestCheck; i: number }[] }[] {
    const out: { group: string; items: { c: TestCheck; i: number }[] }[] = []
    list.forEach((c, i) => {
      const group = c.group ?? ''
      if (out.at(-1)?.group !== group) out.push({ group, items: [] })
      out.at(-1)!.items.push({ c, i })
    })
    return out
  }
  const grouped = (list: TestCheck[]) => list.some((c) => c.group)

  const last = $derived(plan?.last ?? null)
  const newCommits = $derived((plan?.changes ?? []).reduce((n, c) => n + c.commits.length, 0))

  let choice = $state<VerdictOption | null>(null)
  let moreMoves = $state(false)
  const mainVerdicts = $derived((verdicts ?? []).filter((v) => v.outcome !== 'moved'))
  const otherVerdicts = $derived((verdicts ?? []).filter((v) => v.outcome === 'moved'))
  let withChecks = $state(true)
  let files = $state<{ name: string; data: string; url: string }[]>([])
  let dropping = $state(false)
  function addFiles(list: FileList | File[] | null | undefined) {
    for (const file of list ?? []) {
      const reader = new FileReader()
      reader.onload = () => {
        const url = String(reader.result)
        const name = file.name && file.name !== 'image.png' ? file.name : `screenshot-${files.length + 1}.png`
        files = [...files, { name, data: url.slice(url.indexOf(',') + 1), url }]
      }
      reader.readAsDataURL(file)
    }
  }
  function onPaste(e: ClipboardEvent) {
    const images = [...(e.clipboardData?.files ?? [])].filter((f) => f.type.startsWith('image/'))
    if (images.length) {
      e.preventDefault()
      addFiles(images)
    }
  }
  let note = $state('')
  let thenBack = $state(true)
  let recording = $state(false)
  let justRecorded = $state('')
  const recorded = $derived(justRecorded || (session?.verdict ? `${id} moved to ${session.verdict.to}` : ''))
  // A back to main that left repos behind, kept on this page so it isn't missed.
  const stuck = $derived(session && tester.finished?.key === id && !tester.finished.outcome.done ? tester.finished.outcome.steps.filter((s) => !s.ok) : [])
  const noteLabel = $derived(choice?.outcome === 'failed' ? 'What went wrong?' : choice?.outcome === 'passed' ? 'What did you check? (optional)' : 'Note (optional)')
  const canRecord = $derived(!!choice && (choice.outcome !== 'failed' || note.trim() !== ''))
  const commenting = $derived(note.trim() !== '' || files.length > 0 || (withChecks && checks.length > 0))
  async function record() {
    if (!choice || !canRecord) return
    recording = true
    const msg = await verdict({ id: choice.id, note, withChecks: withChecks && checks.length > 0, files: files.map(({ name, data }) => ({ name, data })) })
    recording = false
    if (!msg) return
    justRecorded = msg
    if (thenBack) await backToMain(msg)
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
  let now = $state(Date.now())
  const clock = setInterval(() => (now = Date.now()), 30_000)
  onDestroy(() => clearInterval(clock))
  const elapsed = $derived(session ? ago(session.started, now) : '')
</script>

<div class="page">
  <header style="--wails-draggable: drag">
    <div class="heading">
      <div class="meta mono">
        {#if url}<button class="link" style="--wails-draggable: no-drag" onclick={() => api.openURL(url)}>{id}<Icon name="external" size={11} /></button>{:else}<span>{id}</span>{/if}
        {#if session}
          <span>·</span><span class="ok-text">testing for {elapsed}</span>{#if tester.pullNote}<span>·</span><span>{tester.pullNote}</span>{/if}<span>·</span><span>{session.repos.length} repo{session.repos.length === 1 ? '' : 's'} on {id}</span>
        {:else if plan?.ticket.status}
          <span>·</span><span class="ok-text">{plan.ticket.status}</span>
          {#if plan.ticket.type || plan.ticket.priority}<span>·</span><span>{[plan.ticket.type, plan.ticket.priority].filter(Boolean).join(' · ')}</span>{/if}
          {#if plan.ticket.assignee}<span>·</span><span>{plan.ticket.assignee}</span>{/if}
        {/if}
        {#if last && !session}
          <span>·</span><span class:warn={last.result === 'failed'}>last tested {ago(last.at, now)} ago: {last.result}</span>
        {/if}
      </div>
      <h1>{starting || lastSteps ? `Switching to ${id}` : title || id}</h1>
    </div>
    <div class="actions" style="--wails-draggable: no-drag">
      {#if session && !starting && !lastSteps}
        <button class="btn" class:primary={behind.length > 0} disabled={tester.pulling} onclick={pullLatest} title="Bring every repo under test up to origin's latest">
          <Icon name="sync" spin={tester.pulling} />Pull latest
        </button>
        <button class="btn" disabled={tester.finishing} onclick={() => backToMain(recorded)}>
          <Icon name="refresh" spin={tester.finishing} />Back to main
        </button>
      {:else if !session && !starting && !lastSteps}
        <button class="btn primary big" disabled={!plan || usable.length === 0 || !!other} onclick={test}>
          <Icon name="branch" />{usable.length > 1 ? `Switch ${usable.length} repos and test` : 'Switch and test'}
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
        {#if session}
          <button class="btn primary" onclick={() => (lastSteps = null)}>Test with the repos that switched</button>
          <button class="btn" disabled={tester.finishing} onclick={() => { lastSteps = null; backToMain() }}>Back to main</button>
        {:else}
          <button class="btn primary" onclick={() => (lastSteps = null)}>Back to the plan</button>
        {/if}
      </div>
    {/if}
  {:else if session}
    {#if behind.length}
      <div class="banner">
        <Icon name="sync" />
        <span class="grow">
          {#each behind as b, i (b.name)}
            {i ? ' · ' : ''}{b.behind} new commit{b.behind === 1 ? '' : 's'} in <span class="mono">{shortRepo(b.name)}</span>{#if b.new.length}: <span class="sub">{b.new.map((m) => `"${m}"`).join(', ')}</span>{/if}
          {/each}
          {#if checked}<span class="sub"> · checks you ticked may need redoing</span>{/if}
        </span>
      </div>
    {/if}
    {#if stuck.length}
      <div class="banner">
        <Icon name="alert" />
        <span class="grow selectable">
          {#each stuck as s, i (s.repo)}{i ? ' · ' : ''}<span class="mono">{shortRepo(s.repo)}</span> couldn't go back: {s.message}{/each}
        </span>
      </div>
    {/if}
    <div class="cols">
      <div class="col">
        <section class="panel checklist" aria-label="Checklist">
          <div class="panel-head">
            <span class="eyebrow">Checklist{checks.length ? ` · ${checked} of ${checks.length}` : ''}</span>
            {#if checks.length}<div class="bar"><div style:width="{(checked / checks.length) * 100}%"></div></div>{/if}
          </div>
          {#if !checks.length}<p class="small muted">Nothing to check yet. Add what you mean to try, and tick it off as you go.</p>{/if}
          <div class="items">
          {#each sections(checks) as sec, si (si)}
            {#if grouped(checks)}<div class="section">{sec.group || 'Your checks'}</div>{/if}
            {#each sec.items as { c, i } (i)}
              <label class="check"><input type="checkbox" checked={c.done} onchange={() => toggleCheck(i)} /><span class:done={c.done}>{c.text}</span></label>
            {/each}
          {/each}
          </div>
          {@render addCheckForm()}
        </section>
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
        {#if tester.buildTargets.length}
          <section class="panel builds" aria-label="Builds">
            <div class="panel-head">
              <span class="eyebrow">Builds · npm install, then npm run build</span>
              {#if tester.buildTargets.length > 1}<button class="btn small" disabled={anyBuilding} onclick={buildAll}>Build all</button>{/if}
            </div>
            {#each tester.buildTargets as t (buildKey(t))}
              {@const b = tester.builds[buildKey(t)]}
              {@const bs = buildState(t)}
              <div class="build">
                <span class="stack grow">
                  <span class="mono">{shortRepo(t.repo)}{t.dir ? `/${t.dir}` : ''}</span>
                  <span class="small {bs.tone}">{bs.text}</span>
                </span>
                {#if b?.lines.length}
                  <button class="link small" onclick={() => (openLog = openLog === buildKey(t) ? '' : buildKey(t))}>{openLog === buildKey(t) ? 'Hide output' : 'Output'}</button>
                {/if}
                {#if b?.running}
                  <button class="btn small" onclick={() => stopBuild(t)}><Icon name="close" size={14} />Stop</button>
                {:else}
                  <button class="btn small" onclick={() => { openLog = buildKey(t); runBuild(t) }}><Icon name="play" size={14} />Build</button>
                {/if}
              </div>
              {#if openLog === buildKey(t) && b?.lines.length}
                <pre class="log selectable" use:scrollEnd={b.lines.length}>{b.lines.join('\n')}</pre>
              {/if}
            {/each}
          </section>
        {/if}
      </div>
      <section class="panel verdict" aria-label="Your verdict">
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
            {#each moreMoves || !mainVerdicts.length ? verdicts : mainVerdicts as v (v.id)}
              <label class="choice {v.outcome}" class:on={choice?.id === v.id}>
                <input type="radio" name="verdict" checked={choice?.id === v.id} onchange={() => (choice = v)} />
                <Icon name={v.outcome === 'passed' ? 'check' : v.outcome === 'failed' ? 'x' : 'send'} />
                <span class="stack"><span class="choice-name">{v.name}</span><span class="small muted">moves it to {v.to}</span></span>
              </label>
            {/each}
            {#if mainVerdicts.length && otherVerdicts.length && !moreMoves}
              <button class="link small more" onclick={() => (moreMoves = true)}>More moves ({otherVerdicts.length})</button>
            {/if}
          </fieldset>
          {#if choice}
            <label class="small sub" for="t-note">{noteLabel}</label>
            <textarea id="t-note" class="input" rows="3" bind:value={note} onpaste={onPaste}
              placeholder={choice.outcome === 'failed' ? 'Steps to reproduce, what you expected, what happened' : 'Browsers, pages or cases you covered'}></textarea>
            <div class="drop" class:dropping role="group" aria-label="Screenshots"
              ondragover={(e) => { e.preventDefault(); dropping = true }} ondragleave={() => (dropping = false)}
              ondrop={(e) => { e.preventDefault(); dropping = false; addFiles(e.dataTransfer?.files) }}>
              {#each files as file, i (i)}
                <div class="file">
                  <img src={file.url} alt="" />
                  <span class="grow mono small">{file.name}</span>
                  <button class="icon-btn" aria-label="Remove {file.name}" onclick={() => (files = files.filter((_, j) => j !== i))}><Icon name="close" size={14} /></button>
                </div>
              {/each}
              <label class="pick small muted">
                <input type="file" accept="image/*,video/*,.log,.txt,.har" multiple onchange={(e) => { addFiles(e.currentTarget.files); e.currentTarget.value = '' }} />
                Paste, drop or <span class="link">choose</span> screenshots to attach
              </label>
            </div>
            {#if checks.length}
              <label class="then"><input type="checkbox" bind:checked={withChecks} />Add the checklist to the comment</label>
            {/if}
            <label class="then">
              <input type="checkbox" bind:checked={thenBack} />
              Then put every repo back on main
            </label>
            <button class="btn record {choice.outcome}" disabled={!canRecord || recording || tester.finishing} onclick={record}>
              <Icon name={recording || tester.finishing ? 'running' : choice.outcome === 'failed' ? 'x' : 'check'} spin={recording || tester.finishing} />
              {choice.name}: move to {choice.to}{thenBack ? ' and go back' : ''}
            </button>
            <span class="small muted">{choice.outcome === 'failed' && !note.trim() ? 'Add what went wrong so the developer can fix it.' : commenting ? 'Added to the ticket as a comment.' : 'Nothing is added to the ticket but the move.'}</span>
          {/if}
        {/if}
      </section>
    </div>
  {:else}
    {#if other}
      <div class="note"><Icon name="alert" color="var(--warn)" /><span class="grow">You're testing <span class="mono">{other.key}</span>. Go back to main before testing another change.</span>
        <button class="btn small" onclick={() => navigate({ name: 'test', id: other.key })}>Open {other.key}</button></div>
    {:else if plan && !plan.repos.length}
      <div class="note"><Icon name="alert" color="var(--warn)" /><span>No open PR names {id} in its branch or title, and none of your clones has a <span class="mono">{id}</span> branch on origin. If it's already merged, test it on main.</span></div>
    {/if}
    {#if plan?.error}<div class="small warn">{plan.error}</div>{/if}
    <div class="cols">
      <div class="col">
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
                <span>Set aside my uncommitted work in {dirty.map((r) => shortRepo(r.name)).join(', ')}</span>
                <span class="small muted">Stashed while you test, and put back when you return to main.</span>
              </span>
            </label>
          {/if}
          {#if uncloneable.length}
            <div class="aside small muted">{uncloneable.length} repo{uncloneable.length === 1 ? '' : 's'} will be skipped: turn on cloning in Settings to include {uncloneable.length === 1 ? 'it' : 'them'}.</div>
          {/if}
        </section>
        {#if last}
          <section class="panel" aria-label="Since your last test">
            <div class="panel-head">
              <span class="eyebrow">Since your last test</span>
              <span class="mono small muted">{newCommits} new commit{newCommits === 1 ? '' : 's'}</span>
            </div>
            {#if last.result === 'failed' && last.note}
              <div class="last-note selectable">You failed it {ago(last.at, now)} ago: “{last.note}”</div>
            {/if}
            {#each plan?.changes ?? [] as c (c.repo)}
              {#if c.error}
                <div class="commit"><span class="mono muted">{shortRepo(c.repo)}</span><span class="warn">{c.error}</span><span></span></div>
              {/if}
              {#each c.commits as m (m.sha)}
                <div class="commit"><span class="mono muted">{shortRepo(c.repo)}</span><span>{m.subject}</span><span class="mono muted">{ago(m.at, now)}</span></div>
              {/each}
            {/each}
            {#if !newCommits && !(plan?.changes ?? []).some((c) => c.error)}
              <p class="small muted">Nothing new has been pushed since you tested it {ago(last.at, now)} ago.</p>
            {/if}
          </section>
        {/if}
      </div>
      <div class="col">
        <section class="panel checklist" aria-label="Checklist">
          <div class="panel-head">
            <span class="eyebrow">Checklist{plan?.checklist.length ? ' · from the ticket' : ''}</span>
            {#if draft?.length}<span class="small muted">{draft.length} item{draft.length === 1 ? '' : 's'}</span>{/if}
          </div>
          {#if !plan}
            <p class="small muted">Reading the ticket…</p>
          {:else}
            {#if !draft?.length}<p class="small muted">The ticket has no list to test against. Add the checks you mean to try.</p>{/if}
            <div class="items">
            {#each sections(draft ?? []) as sec, si (si)}
              {#if grouped(draft ?? [])}
                <div class="section">
                  <span class="grow">{sec.group || 'Your checks'}</span>
                  <button class="link small" onclick={() => { const drop = new Set(sec.items.map((x) => x.i)); draft = (draft ?? []).filter((_, j) => !drop.has(j)) }}>Remove section</button>
                </div>
              {/if}
              {#each sec.items as { c, i } (i)}
                <div class="check"><span class="dot"></span><span class="grow">{c.text}</span>
                  <button class="icon-btn" aria-label="Remove this check" onclick={() => (draft = (draft ?? []).filter((_, j) => j !== i))}><Icon name="close" size={14} /></button></div>
              {/each}
            {/each}
            </div>
            {@render addCheckForm()}
          {/if}
        </section>
        <details class="panel" open={!!plan && !plan.checklist.length}>
          <summary class="eyebrow">Ticket description</summary>
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
        </details>
      </div>
    </div>
  {/if}
</div>

{#snippet addCheckForm()}
  <form class="add" onsubmit={(e) => { e.preventDefault(); addCheck() }}>
    <input class="input" bind:value={newCheck} placeholder="Add a check" aria-label="Add a check" />
    <button class="btn small" disabled={!newCheck.trim()}>Add</button>
  </form>
{/snippet}

<style>
  /* Each column scrolls on its own, so a long checklist never scrolls the page. */
  .page { height: 100%; box-sizing: border-box; overflow-y: auto; padding: 0 32px 24px; display: flex; flex-direction: column; gap: 18px; }
  .page > * { flex-shrink: 0; }
  .page > .cols { flex: 1 1 auto; min-height: 0; }
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
  .cols { display: grid; grid-template-columns: minmax(0, 1fr) 400px; grid-template-rows: minmax(0, 1fr); gap: 16px; align-items: start; }
  .cols > * { max-height: 100%; overflow-y: auto; box-sizing: border-box; }
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
  .col { display: flex; flex-direction: column; gap: 16px; min-width: 0; min-height: 0; }
  .col > * { flex-shrink: 0; }
  .col > .checklist { flex-shrink: 1; min-height: 0; overflow: hidden; }
  /* A long list keeps a few rows in view however much else the column holds. */
  .col > .checklist:has(.items > :nth-child(6)) { min-height: 200px; }
  .items { display: flex; flex-direction: column; gap: 10px; min-height: 0; overflow-y: auto; margin-right: -8px; padding-right: 8px; }
  .panel .bar { width: 120px; }
  .check { display: flex; gap: 10px; align-items: flex-start; font-size: 13.5px; line-height: 1.45; color: var(--text-2); }
  .check input { width: 16px; height: 16px; margin: 2px 0 0; accent-color: var(--accent); flex-shrink: 0; }
  .check .done { color: var(--muted); text-decoration: line-through; }
  .check .dot { width: 6px; height: 6px; margin: 8px 4px 0; border-radius: 3px; background: var(--muted); flex-shrink: 0; }
  .check .icon-btn { width: 22px; height: 22px; margin-top: -1px; }
  .add { display: flex; gap: 8px; }
  .build { display: flex; gap: 12px; align-items: center; font-size: 13px; }
  .log { margin: 0; padding: 10px 12px; background: var(--log-bg); border: 1px solid var(--line); border-radius: 8px; font: 12px/1.6 var(--mono); color: var(--text-2); white-space: pre-wrap; word-break: break-word; max-height: 260px; overflow: auto; }
  .section { display: flex; align-items: baseline; gap: 12px; margin-top: 6px; font-size: 13px; font-weight: 600; color: var(--text); }
  .section:first-of-type { margin-top: 0; }
  .add .input { flex: 1; min-height: 30px; font-size: 13px; }
  .commit { display: grid; grid-template-columns: 120px minmax(0, 1fr) auto; gap: 12px; font-size: 13px; }
  .commit > * { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .last-note { padding: 10px 12px; border-radius: 8px; background: var(--warn-bg); border: 1px solid var(--warn-border); color: var(--warn-text); font-size: 13px; line-height: 1.45; }
  details.panel summary { cursor: pointer; list-style-position: inside; }
  .more { align-self: flex-start; }
  .drop { display: flex; flex-direction: column; gap: 8px; padding: 10px 12px; border: 1px dashed var(--line-2); border-radius: 8px; }
  .drop.dropping { border-color: var(--accent); background: var(--accent-bg); }
  .file { display: flex; gap: 10px; align-items: center; }
  .file img { width: 44px; height: 32px; object-fit: cover; border-radius: 4px; background: var(--line); }
  .pick { cursor: pointer; }
  .pick input { position: absolute; width: 1px; height: 1px; opacity: 0; }
</style>
