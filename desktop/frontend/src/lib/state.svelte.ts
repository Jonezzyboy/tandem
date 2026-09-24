import { api, errorText, on } from './api'
import type { Activity, ChangeSummary, ChangeView, CheckEvent, Inbox, Route } from './types'

export const app = $state({
  route: { name: 'inbox' } as Route,
  changes: [] as ChangeSummary[],
  views: {} as Record<string, ChangeView>,
  inbox: null as Inbox | null,
  inboxLoading: false,
  checks: {} as Record<string, CheckEvent[]>,
  activity: {} as Record<string, Activity[]>,
  composer: false,
  error: '',
})

export function currentId(): string {
  return app.route.name === 'change' ? app.route.id : ''
}

export function navigate(route: Route) {
  app.route = route
  app.composer = false
  const id = currentId()
  api.focus(id)
  if (id) {
    api.change(id).then((v) => { app.views[id] = v }).catch(fail)
  }
  if (route.name === 'inbox') loadInbox(false)
}

export function loadInbox(force: boolean) {
  app.inboxLoading = true
  api.inbox(force)
    .then((i) => { app.inbox = i })
    .catch(fail)
    .finally(() => (app.inboxLoading = false))
}

export function log(id: string, text: string, tone: Activity['tone'] = 'muted') {
  const list = app.activity[id] ?? []
  app.activity[id] = [{ at: Date.now(), text, tone }, ...list].slice(0, 50)
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

export function init() {
  api.changes().then((c) => { app.changes = c }).catch(fail)
  loadInbox(false)
  on<ChangeView>('change', (v) => { app.views[v.id] = v })
  on<ChangeSummary[]>('changes', (c) => { app.changes = c })
  on<Inbox>('inbox', (i) => { app.inbox = i })
  on<CheckEvent>('check', recordCheck)
  on<string>('error', (e) => { app.error = e })
  window.addEventListener('focus', () => api.focus(currentId()))
}
