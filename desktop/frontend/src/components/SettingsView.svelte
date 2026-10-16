<script lang="ts">
  import { onDestroy, untrack } from 'svelte'
  import {
    actions, addRoot, checkUpdate, installUpdate, keycaps, restartToUpdate, languages, notifyKinds, prefs, problem, saveSettings, shortcutFor, shortcutFromEvent, themes,
    upgradeCommand, type NotifyKind, type ThemeId, type TriageSettings,
  } from '@lib/settings.svelte'
  import * as App from '@wailsjs/go/main/App'
  import { api, errorText } from '@lib/api'
  import type { JiraAccount, NotifyStatus, RootInfo } from '@lib/types'
  import { navigate } from '@lib/state.svelte'
  import { ClipboardSetText } from '@wailsjs/runtime/runtime'
  import Avatar from './Avatar.svelte'
  import Kbd from './Kbd.svelte'
  import StatusPicker from './StatusPicker.svelte'
  import Icon from './Icon.svelte'

  let recording = $state<string | null>(null)
  let pending = $state('')
  let recordError = $state('')
  // Custom commands being typed, per language, seeded from saved "cmd:" choices.
  let commands = $state<Record<string, string>>(untrack(() =>
    Object.fromEntries(Object.entries(prefs.settings.editors).filter(([, v]) => v.startsWith('cmd:')).map(([k, v]) => [k, v.slice(4)])),
  ))
  let savedLang = $state('')

  function choiceKind(lang: string): string {
    const c = prefs.settings.editors[lang] ?? ''
    return c.startsWith('cmd:') ? 'cmd' : c
  }

  async function setEditor(lang: string, choice: string) {
    await saveSettings({ editors: { ...prefs.settings.editors, [lang]: choice } })
    savedLang = lang
    setTimeout(() => (savedLang = ''), 1600)
  }

  let triageTest = $state<{ running: boolean, ok: boolean, text: string }>({ running: false, ok: false, text: '' })

  function saveTriage(next: Partial<TriageSettings>) {
    triageTest = { running: false, ok: false, text: '' }
    saveSettings({ triage: { ...prefs.settings.triage, ...next } })
  }

  async function testTriage() {
    // A cold model loads into memory first, which can take several seconds.
    triageTest = { running: true, ok: false, text: 'Testing…' }
    try {
      triageTest = { running: false, ok: true, text: await App.TestTriage(prefs.settings.triage as never) }
    } catch (e) {
      triageTest = { running: false, ok: false, text: errorText(e) }
    }
  }

  function onPick(lang: string, value: string) {
    setEditor(lang, value === 'cmd' ? `cmd:${commands[lang] ?? ''}` : value)
  }

  const scopes = [
    { id: 'app', label: 'Anywhere' },
    { id: 'change', label: 'On a change' },
  ] as const

  function startRecording(id: string) {
    recording = id
    pending = ''
    recordError = ''
    prefs.recording = true
  }

  function stopRecording() {
    recording = null
    pending = ''
    prefs.recording = false
  }

  function onKey(e: KeyboardEvent) {
    if (!recording) return
    e.preventDefault()
    e.stopPropagation()
    if (e.key === 'Escape') {
      stopRecording()
      return
    }
    const s = shortcutFromEvent(e)
    if (!s) return
    pending = s
    recordError = problem(recording, s)
    if (!recordError) {
      saveSettings({ keys: { ...prefs.settings.keys, [recording]: s } })
      stopRecording()
    }
  }

  function reset(id: string) {
    const keys = { ...prefs.settings.keys }
    delete keys[id]
    saveSettings({ keys })
  }

  function resetAll() {
    saveSettings({ keys: {} })
  }

  onDestroy(() => {
    prefs.recording = false
  })

  let jiraAccount = $state<JiraAccount | null>(null)
  let jiraEmail = $state(untrack(() => prefs.settings.jira?.email ?? ''))
  let jiraToken = $state('')
  let jiraNote = $state<{ ok: boolean; text: string }>({ ok: false, text: '' })
  api.jiraAccount().then((a) => (jiraAccount = a))

  async function saveJira() {
    try {
      jiraAccount = await api.saveJira(jiraEmail, jiraToken)
      // SaveJira writes settings itself; keep the copy here in step so later saves don't revert it.
      prefs.settings = { ...prefs.settings, jira: { ...prefs.settings.jira, email: jiraAccount.email } }
      jiraToken = ''
      jiraNote = { ok: true, text: 'Saved' }
    } catch (e) {
      jiraNote = { ok: false, text: errorText(e) }
    }
  }

  function saveTester(next: Partial<typeof prefs.settings.tester>) {
    saveSettings({ tester: { ...prefs.settings.tester, ...next } })
  }

  let jiraStatuses = $state<string[] | null>(null)
  let jiraStatusesError = $state('')
  $effect(() => {
    if (!prefs.settings.jira.site) return
    jiraStatusesError = ''
    api.jiraStatuses().then((s) => (jiraStatuses = s)).catch((e) => { jiraStatuses = []; jiraStatusesError = errorText(e) })
  })

  const customised = $derived(Object.keys(prefs.settings.keys).length)

  let roots = $state<RootInfo[] | null>(null)
  const loadRoots = () => api.codeRoots().then((r) => (roots = r))
  loadRoots()

  async function pickRoot() {
    if (await addRoot()) loadRoots()
  }

  async function setRoots(next: string[]) {
    await saveSettings({ roots: next })
    loadRoots()
  }

  let notifyStatus = $state<NotifyStatus | null>(null)
  let notifyRefused = $state(false)
  let notifyTest = $state<{ running: boolean; ok: boolean; text: string }>({ running: false, ok: false, text: '' })

  async function testNotification() {
    notifyTest = { running: true, ok: false, text: 'Sending…' }
    try {
      notifyTest = { running: false, ok: true, text: await api.testNotification() }
    } catch (e) {
      notifyTest = { running: false, ok: false, text: errorText(e) }
    }
  }
  api.notificationStatus().then((s) => (notifyStatus = s))

  async function allowNotifications() {
    try {
      if (notifyRefused) return await api.openNotificationSettings()
      notifyStatus = await api.allowNotifications()
      notifyRefused = !notifyStatus.allowed
    } catch (e) {
      prefs.error = errorText(e)
    }
  }

  let copiedUpgrade = $state(false)
  async function copyUpgrade() {
    await ClipboardSetText(upgradeCommand)
    copiedUpgrade = true
    setTimeout(() => (copiedUpgrade = false), 1600)
  }

  function setNotify(kind: NotifyKind, on: boolean) {
    saveSettings({ notify: { ...prefs.settings.notify, [kind]: on } })
  }
</script>

<svelte:window onkeydowncapture={onKey} onfocus={() => api.notificationStatus().then((s) => (notifyStatus = s))} />

<div class="page">
  <header style="--wails-draggable: drag">
    <div class="muted mono small">Saved as you change them</div>
    <h1>Settings</h1>
  </header>

  {#if prefs.error}<div class="banner">{prefs.error}</div>{/if}

  <section aria-labelledby="s-mode">
    <h2 id="s-mode">How you use Tandem</h2>
    <fieldset class="modes">
      <legend class="sr-only">Mode</legend>
      <label class="mode" class:on={prefs.settings.mode === 'developer'}>
        <input type="radio" name="mode" checked={prefs.settings.mode === 'developer'} onchange={() => saveSettings({ mode: 'developer' })} />
        <span class="stack">
          <span class="mode-name">Developer</span>
          <span class="small muted">Start changes across repos, open PRs and run the merge train.</span>
        </span>
      </label>
      <label class="mode" class:on={prefs.settings.mode === 'tester'}>
        <input type="radio" name="mode" checked={prefs.settings.mode === 'tester'} onchange={() => saveSettings({ mode: 'tester' })} />
        <span class="stack">
          <span class="mode-name">Tester</span>
          <span class="small muted">See the changes waiting to be tested, put every repo on one with a click, and back on main when you're done.</span>
        </span>
      </label>
    </fieldset>
    {#if prefs.settings.mode === 'tester'}
      <div class="card general tester">
        <div class="statuses">
          <div class="stack">
            <span>Statuses that mean a ticket is ready to test</span>
            <span class="small muted">Tickets in any of these show under Ready to test. The verdicts offered are each ticket's own moves out of its status.</span>
          </div>
          <StatusPicker selected={prefs.settings.tester.statuses} options={jiraStatuses}
            onchange={(next) => saveTester({ statuses: next })} />
          {#if !prefs.settings.jira.site}
            <span class="small muted">Add your Jira site below to choose from its statuses.</span>
          {:else if jiraStatusesError}
            <span class="small warn">Couldn't read Jira's statuses: {jiraStatusesError}</span>
          {:else if !prefs.settings.tester.statuses.length}
            <span class="small warn">None chosen yet, so nothing shows under Ready to test.</span>
          {/if}
        </div>
        <label class="choice">
          <input type="checkbox" checked={prefs.settings.tester.cloneMissing} onchange={(e) => saveTester({ cloneMissing: e.currentTarget.checked })} />
          Clone a change's repos that aren't on this machine yet
        </label>
        {#if !prefs.settings.jira.site}<span class="small warn">Add your Jira site below so Tandem can find tickets to test.</span>{/if}
      </div>
    {/if}
  </section>

  <section aria-labelledby="s-account">
    <h2 id="s-account">Account</h2>
    <div class="card account">
      <Avatar account={prefs.account} size={44} />
      {#if prefs.account && !prefs.account.error}
        <div class="stack">
          <span class="name">{prefs.account.name || prefs.account.login}</span>
          <span class="muted small mono">@{prefs.account.login} · github.com</span>
        </div>
        <span class="muted small note">Tandem uses your <span class="mono">gh</span> login. Switch accounts with <span class="mono">gh auth switch</span>.</span>
      {:else}
        <div class="stack">
          <span class="name">Not signed in</span>
          <span class="warn small">{prefs.account?.error ?? 'Checking gh…'}</span>
        </div>
      {/if}
    </div>
  </section>

  <section aria-labelledby="s-roots">
    <div class="section-head">
      <h2 id="s-roots">Repositories</h2>
      {#if prefs.settings.roots.length}<button class="btn small" onclick={() => setRoots([])}>Use the default</button>{/if}
    </div>
    <div class="card general">
      <p class="muted small intro">Folders holding your clones as <span class="mono">&lt;owner&gt;/&lt;repo&gt;</span>. New changes pick repos from here.{' '}{#if !prefs.settings.roots.length}This is the default; <span class="mono">td</span> uses it too.{:else}<span class="mono">td</span> reads <span class="mono">$TANDEM_ROOT</span> instead.{/if}</p>
      <ul class="roots">
        {#each roots ?? [] as r (r.path)}
          <li>
            <Icon name="folder" color="var(--muted)" />
            <span class="mono grow selectable">{r.path}</span>
            <span class="small" class:muted={r.repos > 0} class:warn={r.repos === 0}>{r.repos === 1 ? '1 repo' : `${r.repos} repos`}</span>
            <button class="icon-btn" aria-label="Show {r.path} in Finder" title="Show in Finder" onclick={() => api.openFolder(r.path)}><Icon name="external" /></button>
            {#if prefs.settings.roots.length}
              <button class="icon-btn" aria-label="Stop searching {r.path}" title="Remove" onclick={() => setRoots(prefs.settings.roots.filter((p) => p !== r.path))}><Icon name="trash" /></button>
            {/if}
          </li>
        {/each}
      </ul>
      <div><button class="btn small" onclick={pickRoot}><Icon name="plus" />Add folder</button></div>
    </div>
  </section>

  <section aria-labelledby="s-jira">
    <h2 id="s-jira">Jira</h2>
    <div class="card general">
      <p class="muted small intro">Lets a pasted ticket link fill in the change’s title and show the ticket’s status. Without it, the link is still kept.</p>
      <label class="setting">
        <span>Jira site</span>
        <input class="input mono" placeholder="https://acme.atlassian.net" value={prefs.settings.jira.site}
          onchange={(e) => saveSettings({ jira: { ...prefs.settings.jira, site: e.currentTarget.value } })} />
      </label>
      <label class="setting">
        <span>Atlassian account email</span>
        <input class="input" type="email" placeholder="you@company.com" bind:value={jiraEmail} autocomplete="off" />
      </label>
      <label class="setting">
        <span>API token <span class="muted small">· kept in the Keychain</span></span>
        <input class="input mono" type="password" bind:value={jiraToken} autocomplete="off"
          placeholder={jiraAccount?.hasToken ? 'Saved; paste a new one to replace it' : 'Paste a token'} />
      </label>
      <div class="triage-test">
        <button class="btn small" disabled={!jiraEmail.trim()} onclick={saveJira}>Save</button>
        <button class="link small" onclick={() => api.openURL('https://id.atlassian.com/manage-profile/security/api-tokens')}>Create a token</button>
        {#if jiraNote.text}
          <span class="small" class:ok={jiraNote.ok} class:warn={!jiraNote.ok} role="status">{jiraNote.text}</span>
        {:else if jiraAccount && !jiraAccount.hasToken}
          <span class="small muted">No token saved</span>
        {/if}
      </div>
      {#if prefs.settings.mode === 'developer'}
        {#each [{ key: 'onPRs', label: 'When a change\'s PRs open, move its ticket to' }, { key: 'onMerged', label: 'When the merge train finishes, move it to' }] as f (f.key)}
          {@const current = prefs.settings.jira[f.key as 'onPRs' | 'onMerged']}
          <label class="setting">
            <span>{f.label}</span>
            <select class="input" value={current} disabled={!prefs.settings.jira.site}
              onchange={(e) => saveSettings({ jira: { ...prefs.settings.jira, [f.key]: e.currentTarget.value } })}>
              <option value="">Leave it where it is</option>
              {#if current && !(jiraStatuses ?? []).includes(current)}<option value={current}>{current}</option>{/if}
              {#each jiraStatuses ?? [] as st (st)}<option value={st}>{st}</option>{/each}
            </select>
          </label>
        {/each}
      {/if}
    </div>
  </section>

  <section aria-labelledby="s-theme">
    <h2 id="s-theme">Appearance</h2>
    <div class="themes" role="radiogroup" aria-labelledby="s-theme">
      {#each themes as t (t.id)}
        {@const preview = t.id === 'system' ? (prefs.systemDark ? 'graphite' : 'paper') : t.id}
        <button
          class="theme"
          class:on={prefs.settings.theme === t.id}
          role="radio"
          aria-checked={prefs.settings.theme === t.id}
          onclick={() => saveSettings({ theme: t.id as ThemeId })}
        >
          <div class="preview" data-theme={preview}>
            <div class="p-nav">
              <span class="p-line"></span><span class="p-line short"></span><span class="p-line"></span>
            </div>
            <div class="p-main">
              <span class="p-title"></span>
              <div class="p-row"><span class="p-dot ok"></span><span class="p-line"></span></div>
              <div class="p-row warnrow"><span class="p-dot warn"></span><span class="p-line"></span></div>
              <span class="p-button"></span>
            </div>
          </div>
          <div class="theme-label">
            <span>{t.name}</span>
            <span class="muted small">{t.note}</span>
          </div>
        </button>
      {/each}
    </div>
  </section>

  <section aria-labelledby="s-keys">
    <div class="section-head">
      <h2 id="s-keys">Keyboard shortcuts</h2>
      {#if customised}<button class="btn small" onclick={resetAll}>Reset all to defaults</button>{/if}
    </div>
    {#each scopes as scope}
      <div class="card keys">
        <div class="eyebrow">{scope.label}</div>
        {#each actions.filter((a) => a.scope === scope.id) as a (a.id)}
          {@const custom = prefs.settings.keys[a.id] !== undefined}
          <div class="key-row" class:recording={recording === a.id}>
            <span class="grow">{a.label}</span>
            {#if recording === a.id}
              <span class="small" class:warn={recordError} class:muted={!recordError}>
                {recordError || 'Press the new shortcut · Esc to cancel'}
              </span>
              {#if pending}<Kbd keys={keycaps(pending)} />{/if}
            {:else}
              <Kbd keys={keycaps(shortcutFor(a.id))} />
            {/if}
            <button class="btn small" onclick={() => (recording === a.id ? stopRecording() : startRecording(a.id))}>
              {recording === a.id ? 'Cancel' : 'Change'}
            </button>
            <button class="btn small" disabled={!custom} onclick={() => reset(a.id)} aria-label="Reset {a.label} to its default">Reset</button>
          </div>
        {/each}
        {#if scope.id === 'app'}
          <div class="key-row fixed">
            <span class="grow">Jump to a change in the sidebar</span>
            <Kbd keys={['⌘', '1']} /><span class="muted small">to</span><Kbd keys={['⌘', '9']} />
          </div>
        {/if}
      </div>
    {/each}
  </section>

  <section aria-labelledby="s-editors">
    <h2 id="s-editors">Editors</h2>
    <div class="card editors">
      <p class="muted small intro">“Open in editor” picks by the repo’s language: <span class="mono">go.mod</span> is Go, <span class="mono">composer.json</span> PHP, <span class="mono">package.json</span> JavaScript. Files at the repo root win over subfolders.</p>
      {#each languages as lang (lang.id)}
        {@const kind = choiceKind(lang.id)}
        {@const chosen = prefs.editorApps.find((a) => a.id === kind)}
        <div class="editor-row">
          <label class="editor-label" for="ed-{lang.id}">{lang.id === 'other' ? 'Other' : lang.label}
            {#if lang.id === 'other'}<span class="muted small">{lang.label}</span>{/if}
          </label>
          <div class="editor-pick">
            <select id="ed-{lang.id}" class="input" value={kind} onchange={(e) => onPick(lang.id, e.currentTarget.value)}>
              {#each prefs.editorApps.filter((a) => a.installed) as app (app.id)}
                <option value={app.id}>{app.name}</option>
              {/each}
              {#if chosen && !chosen.installed}<option value={chosen.id}>{chosen.name} (not installed)</option>{/if}
              <option value="cmd">Custom command…</option>
            </select>
            {#if kind === 'cmd'}
              <input class="input mono" aria-label="{lang.label} editor command" placeholder={lang.id === 'other' ? 'blank: code' : 'e.g. cursor'}
                bind:value={commands[lang.id]} onchange={() => setEditor(lang.id, `cmd:${commands[lang.id] ?? ''}`)}
                onkeydown={(e) => e.key === 'Enter' && setEditor(lang.id, `cmd:${commands[lang.id] ?? ''}`)} />
            {/if}
            {#if chosen && !chosen.installed}<span class="warn small">not installed: falls back to Other</span>{/if}
            {#if savedLang === lang.id}<span class="ok small">Saved</span>{/if}
          </div>
        </div>
      {/each}
    </div>
  </section>

  <section aria-labelledby="s-general">
    <h2 id="s-general">General</h2>
    <div class="card general">
      <fieldset class="field">
        <legend>Merge train default</legend>
        <div class="choices">
          {#each ['squash', 'merge', 'rebase'] as m}
            <label class="choice">
              <input type="radio" name="merge-method" value={m} checked={prefs.settings.mergeMethod === m}
                onchange={() => saveSettings({ mergeMethod: m as 'squash' | 'merge' | 'rebase' })} />{m}
            </label>
          {/each}
        </div>
      </fieldset>
      <label class="choice">
        <input type="checkbox" checked={prefs.settings.draftPRs} onchange={(e) => saveSettings({ draftPRs: e.currentTarget.checked })} />
        Open new PRs as drafts by default
      </label>
    </div>
  </section>

  <section aria-labelledby="s-notifications">
    <h2 id="s-notifications">Notifications</h2>
    <div class="card general">
      {#if notifyStatus && !notifyStatus.available}
        <p class="muted small intro">Notifications need the installed app; this build can't send them.</p>
      {:else if notifyStatus && !notifyStatus.allowed}
        <div class="triage-test">
          <span class="warn small">macOS isn't letting Tandem notify you.</span>
          <button class="btn small" onclick={allowNotifications}>{notifyRefused ? 'Open System Settings' : 'Allow notifications'}</button>
        </div>
      {/if}
      <p class="muted small intro">Sent while you're elsewhere. Changes are rechecked on GitHub every few minutes, and the one on screen every minute.</p>
      {#each notifyKinds as k (k.id)}
        <label class="choice">
          <input type="checkbox" checked={prefs.settings.notify[k.id]} onchange={(e) => setNotify(k.id, e.currentTarget.checked)} />
          {k.label}
        </label>
      {/each}
      <div class="triage-test">
        <button class="btn small" disabled={!notifyStatus?.allowed || notifyTest.running} onclick={testNotification}>Send a test notification</button>
        {#if notifyTest.text}
          <span class="small" class:ok={notifyTest.ok} class:warn={!notifyTest.ok && !notifyTest.running} class:muted={notifyTest.running} role="status">{notifyTest.text}</span>
        {/if}
      </div>
      <p class="muted small intro">
        Banners, sounds and Focus are set in macOS:
        <button class="link" onclick={() => api.openNotificationSettings().catch((e) => (prefs.error = errorText(e)))}>System Settings → Notifications → Tandem</button>.
      </p>
    </div>
  </section>

  <section aria-labelledby="s-triage">
    <h2 id="s-triage">Merge train triage</h2>
    <div class="card general">
      <label class="choice">
        <input type="checkbox" checked={prefs.settings.triage.enabled} onchange={(e) => saveTriage({ enabled: e.currentTarget.checked })} />
        When a leg's CI fails, ask a local model whether it's flaky and rerun it if so
      </label>
      <label class="setting">
        <span class="muted">Ollama model</span>
        <input class="input mono" placeholder="nimble" value={prefs.settings.triage.model}
          disabled={!prefs.settings.triage.enabled} onchange={(e) => saveTriage({ model: e.currentTarget.value })} />
      </label>
      <label class="setting">
        <span class="muted">Ollama server</span>
        <input class="input mono" placeholder="http://localhost:11434" value={prefs.settings.triage.url}
          disabled={!prefs.settings.triage.enabled} onchange={(e) => saveTriage({ url: e.currentTarget.value })} />
      </label>
      <label class="setting">
        <span class="muted">Rerun only when at least this sure</span>
        <input class="input mono" type="number" min="0.5" max="1" step="0.05" value={prefs.settings.triage.minConfidence}
          disabled={!prefs.settings.triage.enabled} onchange={(e) => saveTriage({ minConfidence: e.currentTarget.valueAsNumber })} />
      </label>
      <label class="setting">
        <span class="muted">Reruns per leg</span>
        <input class="input mono" type="number" min="1" max="3" step="1" value={prefs.settings.triage.retries}
          disabled={!prefs.settings.triage.enabled} onchange={(e) => saveTriage({ retries: e.currentTarget.valueAsNumber })} />
      </label>
      <div class="triage-test">
        <button class="btn small" disabled={!prefs.settings.triage.enabled || triageTest.running} onclick={testTriage}>Test connection</button>
        {#if triageTest.text}
          <span class="small" class:ok={triageTest.ok} class:warn={!triageTest.ok && !triageTest.running} class:muted={triageTest.running}
            role="status">{triageTest.text}</span>
        {/if}
      </div>
    </div>
  </section>

  <section aria-labelledby="s-updates">
    <h2 id="s-updates">Updates</h2>
    <div class="card updates">
      <div class="update-head">
        <div class="stack">
          <span class="version">Tandem <span class="mono">{prefs.version || '…'}</span></span>
          <span class="small" class:ok={prefs.update?.newer || prefs.install.phase === 'ready'} class:warn={!!prefs.updateError && !prefs.update} class:muted={!prefs.update?.newer && prefs.install.phase !== 'ready'} role="status">
            {#if prefs.install.phase === 'ready'}{prefs.install.version} is installed. Restart to start using it.
            {:else if prefs.install.phase === 'installing'}Installing {prefs.install.version}…
            {:else if prefs.updateChecking}Checking for updates…
            {:else if prefs.update?.newer}{prefs.update.version} is available.
            {:else if prefs.update}Up to date{prefs.version === 'dev' ? ' (dev build)' : ''}{prefs.updateCheckedAt ? ` · checked ${new Date(prefs.updateCheckedAt).toLocaleTimeString([], { hour: 'numeric', minute: '2-digit' })}` : ''}
            {:else if prefs.updateError}Couldn't check for updates: {prefs.updateError}
            {:else}Checking for updates…{/if}
          </span>
        </div>
        <div class="update-actions">
          {#if prefs.update?.newer && prefs.install.phase !== 'ready'}
            <button class="link small" onclick={() => api.openURL(prefs.update!.url)}>What's new</button>
          {/if}
          {#if prefs.install.phase === 'ready'}
            <button class="btn small primary" onclick={restartToUpdate}>Restart to update</button>
          {:else if prefs.install.phase === 'installing'}
            <button class="btn small" disabled><Icon name="running" size={14} spin />Installing</button>
          {:else if prefs.update?.newer}
            <button class="btn small primary" onclick={installUpdate}>Update now</button>
          {:else}
            <button class="btn small" disabled={prefs.updateChecking} onclick={checkUpdate}><Icon name="refresh" size={14} spin={prefs.updateChecking} />Check for updates</button>
          {/if}
        </div>
      </div>
      {#if prefs.install.phase === 'installing' && prefs.install.line}
        <div class="mono small muted install-line">{prefs.install.line}</div>
      {:else if prefs.install.phase === 'failed' || (prefs.install.phase === 'ready' && prefs.install.line)}
        <div class="install-error">
          <span class="small warn">{prefs.install.line}</span>
          {#if prefs.install.phase === 'failed'}
            <span class="small muted">You can also run <button class="link mono" title="Copy" onclick={copyUpgrade}>{upgradeCommand}</button>{copiedUpgrade ? ' · copied' : ''} in Terminal.</span>
          {/if}
        </div>
      {/if}
      <label class="toggle-row">
        <span class="stack">
          <span>Install updates automatically</span>
          <span class="small muted">New versions install in the background and take effect when Tandem restarts.</span>
        </span>
        <input type="checkbox" class="switch" checked={prefs.settings.autoUpdate} onchange={(e) => saveSettings({ autoUpdate: e.currentTarget.checked })} />
      </label>
    </div>
  </section>

  <section aria-labelledby="s-about">
    <h2 id="s-about">About</h2>
    <div class="card about">
      <div class="about-row"><span class="muted">Settings file</span><span class="mono path selectable">{prefs.path}</span></div>
      <div class="about-row"><span class="muted">Change records</span><span class="mono path selectable">{prefs.home}</span></div>
      <div class="about-row">
        <span class="muted">Setup checklist</span>
        <span><button class="btn small" onclick={() => navigate({ name: 'setup' })}>Open</button></span>
      </div>
    </div>
  </section>
</div>

<style>
  .page { padding: 0 32px 40px; display: flex; flex-direction: column; gap: 28px; max-width: 1000px; }
  header { padding-top: 28px; }
  h1 { margin: 4px 0 0; font-family: var(--display); font-weight: 700; font-size: 30px; }
  h2 { margin: 0 0 12px; font-size: 15px; font-weight: 600; }
  .small { font-size: 12px; }
  .banner { padding: 10px 14px; border-radius: 10px; background: var(--warn-bg); border: 1px solid var(--warn-border); color: var(--warn-text); font-size: 13px; }
  .section-head { display: flex; justify-content: space-between; align-items: center; margin-bottom: 12px; }
  .section-head h2 { margin: 0; }
  .card { padding: 16px 18px; background: var(--panel); border: 1px solid var(--line); border-radius: 12px; }
  .account { display: flex; align-items: center; gap: 14px; }
  .stack { display: flex; flex-direction: column; gap: 2px; }
  .name { font-size: 15px; font-weight: 600; }
  .note { margin-left: auto; text-align: right; max-width: 340px; line-height: 1.5; }
  .note .mono, .muted .mono { white-space: nowrap; }

  .themes { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 14px; }
  .theme {
    display: flex; flex-direction: column; gap: 10px; padding: 10px; border-radius: 14px; text-align: left;
    background: var(--panel); border: 1px solid var(--line); cursor: pointer; color: var(--text);
  }
  .theme:hover { border-color: var(--line-2); }
  .theme.on { border-color: var(--accent); box-shadow: 0 0 0 1px var(--accent); }
  .theme-label { display: flex; flex-direction: column; gap: 2px; padding: 0 4px 2px; font-size: 14px; }
  .preview { display: flex; height: 96px; border-radius: 9px; overflow: hidden; border: 1px solid var(--line); background: var(--bg); }
  .p-nav { width: 30%; background: var(--nav); border-right: 1px solid var(--line); display: flex; flex-direction: column; gap: 7px; padding: 12px 8px; }
  .p-main { flex: 1; display: flex; flex-direction: column; gap: 7px; padding: 10px; }
  .p-line { height: 5px; border-radius: 3px; background: var(--line-2); flex: 1; }
  .p-nav .p-line { flex: none; }
  .p-line.short { width: 60%; }
  .p-title { height: 8px; width: 55%; border-radius: 3px; background: var(--text); }
  .p-row { display: flex; align-items: center; gap: 6px; padding: 5px 6px; border-radius: 5px; background: var(--panel); border: 1px solid var(--line); }
  .p-row.warnrow { background: var(--warn-row); }
  .p-dot { width: 7px; height: 7px; border-radius: 4px; flex-shrink: 0; }
  .p-dot.ok { background: var(--ok); }
  .p-dot.warn { background: var(--warn); }
  .p-button { margin-top: auto; align-self: flex-end; width: 34%; height: 11px; border-radius: 4px; background: var(--accent); }

  .keys { display: flex; flex-direction: column; gap: 4px; margin-bottom: 12px; }
  .keys .eyebrow { margin-bottom: 6px; }
  .key-row { display: flex; align-items: center; gap: 10px; min-height: 40px; padding: 0 8px; border-radius: 8px; }
  .key-row.recording { background: var(--accent-bg); }
  .key-row.fixed { color: var(--text-2); }
  .grow { flex: 1; }

  .general { display: flex; flex-direction: column; gap: 18px; }
  .editors { display: flex; flex-direction: column; gap: 4px; }
  .editors .intro { margin: 0 0 8px; line-height: 1.5; }
  .editors .intro .mono { white-space: nowrap; }
  .editor-row { display: grid; grid-template-columns: 220px minmax(0, 1fr); gap: 16px; align-items: center; min-height: 48px; }
  .editor-label { display: flex; flex-direction: column; gap: 2px; font-size: 14px; }
  .editor-pick { display: flex; align-items: center; gap: 10px; }
  .editor-pick select { width: 240px; }
  .editor-pick .input.mono { width: 220px; }
  fieldset.field { border: 0; margin: 0; padding: 0; }
  legend { font-size: 13px; color: var(--text-2); margin-bottom: 8px; }
  .choices { display: flex; gap: 18px; }
  .triage-test { display: flex; align-items: center; gap: 12px; flex-wrap: wrap; }
  .setting { display: grid; grid-template-columns: 240px minmax(0, 320px); align-items: center; gap: 12px; font-size: 13px; }
  @media (max-width: 640px) { .setting { grid-template-columns: 1fr; gap: 6px; } }
  .choice { display: flex; align-items: center; gap: 8px; font-size: 14px; cursor: pointer; }
  .choice input { accent-color: var(--accent); width: 16px; height: 16px; }

  .intro { margin: 0; line-height: 1.5; }
  .link { border: 0; background: none; padding: 0; color: var(--accent-text); cursor: pointer; }
  .link:hover { color: var(--link-hover); }
  .modes { border: 0; margin: 0 0 12px; padding: 0; display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 14px; }
  .mode { display: flex; gap: 12px; align-items: flex-start; padding: 16px; border-radius: 12px; background: var(--panel); border: 1px solid var(--line); cursor: pointer; }
  .mode.on { background: var(--accent-bg); border-color: var(--accent); }
  .mode input { width: 16px; height: 16px; margin: 3px 0 0; accent-color: var(--accent); }
  .mode-name { font-weight: 500; font-size: 14px; }
  .mode .small { line-height: 1.5; }
  .statuses { display: flex; flex-direction: column; gap: 10px; font-size: 14px; }
  .sr-only { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0 0 0 0); }
  .roots { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 6px; }
  .roots li { display: flex; align-items: center; gap: 10px; min-height: 36px; padding: 0 4px 0 10px; border: 1px solid var(--line); border-radius: 8px; font-size: 13px; }
  .roots li .grow { min-width: 0; overflow-wrap: anywhere; }
  .updates { display: flex; flex-direction: column; gap: 14px; }
  .update-head { display: flex; align-items: center; justify-content: space-between; gap: 16px; }
  .version { font-size: 15px; font-weight: 600; }
  .version .mono { font-weight: 500; }
  .update-actions { display: flex; align-items: center; gap: 14px; flex-shrink: 0; }
  .install-line { overflow-wrap: anywhere; padding: 8px 10px; border-radius: 8px; background: var(--nav); }
  .install-error { display: flex; flex-direction: column; gap: 4px; }
  .toggle-row { display: flex; align-items: center; justify-content: space-between; gap: 16px; padding-top: 14px; border-top: 1px solid var(--line); font-size: 14px; cursor: pointer; }
  .switch {
    appearance: none; flex-shrink: 0; width: 34px; height: 20px; border-radius: 999px; position: relative; cursor: pointer;
    background: var(--line-2); transition: background 0.15s;
  }
  .switch::after {
    content: ''; position: absolute; top: 2px; left: 2px; width: 16px; height: 16px; border-radius: 50%;
    background: var(--text); transition: transform 0.15s;
  }
  .switch:checked { background: var(--accent); }
  .switch:checked::after { transform: translateX(14px); background: var(--on-accent); }
  .switch:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
  .about { display: flex; flex-direction: column; font-size: 13px; padding-top: 6px; padding-bottom: 6px; }
  .about-row { display: grid; grid-template-columns: 180px minmax(0, 1fr); gap: 12px; align-items: center; min-height: 40px; }
  .about-row + .about-row { border-top: 1px solid var(--line); }
  .path { font-size: 12px; color: var(--text-2); overflow-wrap: anywhere; }
</style>
