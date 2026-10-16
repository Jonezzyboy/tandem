import { api, errorText, on } from './api'
import { prefs, saveSettings } from './settings.svelte'
import type { Activity, ChangeSummary, ChangeView, CheckEvent, CleanItem, Inbox, Route, TrainEvent, TrainRun } from './types'

export const app = $state({
  route: { name: 'inbox' } as Route,
  changes: [] as ChangeSummary[],
  views: {} as Record<string, ChangeView>,
  inbox: null as Inbox | null,
  inboxLoading: false,
  checks: {} as Record<string, CheckEvent[]>,
  activity: {} as Record<string, Activity[]>,
  composer: false,
  trainOpen: false,
  palette: false,
  trains: {} as Record<string, TrainRun>,
  cleanPlan: null as CleanItem[] | null,
  cleanLoading: false,
  error: '',
})

// activeChanges are the changes still in flight, in the user's order: the
// sidebar's draggable list and the ⌘1–⌘9 jumps.
export function activeChanges() {
  return app.changes.filter((c) => !c.merged)
}

export function currentId(): string {
  return app.route.name === 'change' ? app.route.id : ''
}

export function navigate(route: Route) {
  app.route = route
  app.composer = false
  app.trainOpen = false
  const id = currentId()
  api.focus(id)
  if (id) {
    api.change(id).then((v) => { app.views[id] = v }).catch(fail)
  }
  if (route.name === 'inbox') {
    loadInbox(false)
    loadCleanPlan()
  }
}

// Change ids with a switch in flight, so every button for them shows it.
export const switching = $state<Record<string, boolean>>({})

// checkOut puts every repo of a branch-based change onto its branch (toBase:
// back onto each repo's base), logging each repo's outcome.
export async function checkOut(id: string, toBase = false) {
  switching[id] = true
  try {
    const results = await api.switchTo(id, toBase)
    for (const r of results) log(id, `${r.leg}: ${r.message}`, r.ok ? 'muted' : 'warn')
    const stuck = results.filter((r) => !r.ok)
    if (stuck.length) fail(`${stuck.map((r) => r.leg).join(', ')} stayed put: ${stuck[0].message.replace(/^stayed put: /, '')}`)
    await api.focus(currentId())
  } catch (e) {
    fail(e)
  } finally {
    switching[id] = false
  }
}

export function loadCleanPlan() {
  app.cleanLoading = true
  api.cleanPlan()
    .then((p) => { app.cleanPlan = p })
    .catch(fail)
    .finally(() => (app.cleanLoading = false))
}

function recordTrain(ev: TrainEvent) {
  const run = app.trains[ev.change] ?? { running: true, events: [], error: '' }
  run.events = [...run.events, ev]
  app.trains[ev.change] = run
  const where = ev.leg ? `${ev.leg}: ` : ''
  log(ev.change, `Train · ${where}${ev.phase} ${ev.detail}`.trim(), ev.phase === 'merged' || ev.phase === 'done' ? 'ok' : 'muted')
}

export function loadInbox(force: boolean) {
  app.inboxLoading = true
  api.inbox(force)
    .then((i) => { app.inbox = i })
    .catch(fail)
    .finally(() => (app.inboxLoading = false))
}

// log adds to a change's activity and its saved history; entries the backend
// sends were saved there already.
export function log(id: string, text: string, tone: Activity['tone'] = 'muted', saved = false) {
  const list = app.activity[id] ?? []
  app.activity[id] = [{ at: Date.now(), text, tone }, ...list].slice(0, 300)
  if (!saved) api.record(id, text, tone)
}

export async function loadHistory(id: string) {
  const entries = await api.history(id)
  app.activity[id] = entries.slice(0, 300).map((e) => ({ at: Date.parse(e.at), text: e.text, tone: e.tone }))
}

export function fail(e: unknown) {
  app.error = errorText(e)
}

// A check's events replace its earlier entry, so the panel shows one row per
// check that moves from running to its result.
function recordCheck(ev: CheckEvent) {
  const list = app.checks[ev.change] ?? []
  const i = list.findIndex((c) => c.leg === ev.leg && c.name === ev.name)
  if (i >= 0) list[i] = ev
  else list.push(ev)
  app.checks[ev.change] = list
}

// looksShared tells a developer's tester link from a Jira link or key.
export function looksShared(text: string): boolean {
  const t = text.trim()
  return t.startsWith('tandem://') || t.includes('github.io/tandem/test') || /^[A-Za-z0-9_-]{40,}$/.test(t)
}

// openShared opens a developer's tester link on its tester page, switching
// to tester mode for it.
export async function openShared(text: string) {
  try {
    const { key } = await api.openShare(text)
    if (prefs.settings.mode !== 'tester') await saveSettings({ mode: 'tester' })
    navigate({ name: 'test', id: key })
  } catch (e) {
    fail(e)
  }
}

// takeLink opens the link macOS started or woke the app with, if any.
export async function takeLink() {
  const link = await api.takeShareLink()
  if (link) openShared(link)
}

export function init() {
  api.changes().then((c) => { app.changes = c }).catch(fail)
  loadInbox(false)
  on<ChangeView>('change', (v) => { app.views[v.id] = v })
  on<ChangeSummary[]>('changes', (c) => { app.changes = c })
  on<Inbox>('inbox', (i) => { app.inbox = i })
  on<CheckEvent>('check', recordCheck)
  on<TrainEvent>('train', recordTrain)
  on<{ change: string; text: string; tone: Activity['tone'] }>('activity', (a) => log(a.change, a.text, a.tone, true))
  on<string>('error', (e) => { app.error = e })
  on('share-link', takeLink)
  on<string>('open', (id) => navigate(id ? { name: 'change', id } : { name: 'inbox' }))
  window.addEventListener('focus', () => {
    api.windowActive(true)
    api.focus(currentId())
  })
  window.addEventListener('blur', () => api.windowActive(false))
  api.windowActive(document.hasFocus())
}
