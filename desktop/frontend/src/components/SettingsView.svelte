<script lang="ts">
  import { onDestroy, untrack } from 'svelte'
  import {
    actions, keycaps, languages, prefs, problem, saveSettings, shortcutFor, shortcutFromEvent, themes, type ThemeId,
  } from '@lib/settings.svelte'
  import Avatar from './Avatar.svelte'
  import Kbd from './Kbd.svelte'

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

  const customised = $derived(Object.keys(prefs.settings.keys).length)
</script>

<svelte:window onkeydowncapture={onKey} />

<div class="page">
  <header style="--wails-draggable: drag">
    <div class="muted mono small">Saved as you change them</div>
    <h1>Settings</h1>
  </header>

  {#if prefs.error}<div class="banner">{prefs.error}</div>{/if}

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

  <section aria-labelledby="s-about">
    <h2 id="s-about">About</h2>
    <div class="card about">
      <div><span class="muted">Version</span><span class="mono">{prefs.version || '…'}</span></div>
      <div><span class="muted">Settings file</span><span class="mono selectable">{prefs.path}</span></div>
      <div><span class="muted">Change records</span><span class="mono selectable">{prefs.home}</span></div>
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
  .editor-pick select { width: 240px; appearance: none; padding-right: 30px; cursor: pointer;
    background-image: linear-gradient(45deg, transparent 50%, var(--muted) 50%), linear-gradient(135deg, var(--muted) 50%, transparent 50%);
    background-position: calc(100% - 16px) 16px, calc(100% - 11px) 16px; background-size: 5px 5px; background-repeat: no-repeat; }
  .editor-pick .input.mono { width: 220px; }
  fieldset.field { border: 0; margin: 0; padding: 0; }
  legend { font-size: 13px; color: var(--text-2); margin-bottom: 8px; }
  .choices { display: flex; gap: 18px; }
  .choice { display: flex; align-items: center; gap: 8px; font-size: 14px; cursor: pointer; }
  .choice input { accent-color: var(--accent); width: 16px; height: 16px; }

  .about { display: flex; flex-direction: column; gap: 8px; font-size: 13px; }
  .about div { display: grid; grid-template-columns: 180px minmax(0, 1fr); gap: 12px; }
</style>
