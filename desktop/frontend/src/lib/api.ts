import * as App from '@wailsjs/go/main/App'
import { EventsOn } from '@wailsjs/runtime/runtime'
import type {
  ChangeSummary, ChangeView, CheckEvent, CleanItem, CleanResult, Inbox, LegResult, PinItem, PRPreview, PRRequest,
  RepoInfo, StartItem, TrainPlan,
} from './types'

// The generated bindings type results as model classes; the JSON is plain
// objects, so they are re-typed here as the interfaces the UI uses.
export const api = {
  changes: () => App.Changes() as unknown as Promise<ChangeSummary[]>,
  change: (id: string) => App.Change(id) as unknown as Promise<ChangeView>,
  focus: (id: string) => App.Focus(id),
  refresh: (id: string) => App.Refresh(id),
  sync: (id: string) => App.Sync(id) as unknown as Promise<LegResult[]>,
  check: (id: string, leg = '') => App.Check(id, leg) as unknown as Promise<CheckEvent[]>,
  planPRs: (id: string, req: PRRequest) => App.PlanPRs(id, req as never) as unknown as Promise<PRPreview>,
  publishPRs: (id: string, req: PRRequest) => App.PublishPRs(id, req as never) as unknown as Promise<PRPreview>,
  inbox: (force = false) => App.Inbox(force) as unknown as Promise<Inbox>,
  repos: () => App.Repos() as unknown as Promise<RepoInfo[]>,
  start: (id: string, title: string, repos: string[]) =>
    App.Start({ id, title, repos } as never) as unknown as Promise<StartItem[]>,
  link: (id: string, up: string, down: string) => App.Link(id, up, down),
  openURL: (url: string) => App.OpenURL(url),
  openFolder: (path: string) => App.OpenFolder(path),
  openEditor: (path: string) => App.OpenEditor(path),
  addRepos: (id: string, repos: string[]) => App.AddRepos(id, repos) as unknown as Promise<StartItem[]>,
  removeLeg: (id: string, leg: string) => App.RemoveLeg(id, leg),
  unlink: (id: string, up: string, down: string) => App.Unlink(id, up, down),
  switchTo: (id: string, toBase = false) => App.Switch(id, toBase) as unknown as Promise<LegResult[]>,
  pin: (id: string) => App.Pin(id) as unknown as Promise<PinItem[]>,
  trainPlan: (id: string) => App.TrainPlan(id) as unknown as Promise<TrainPlan>,
  startTrain: (id: string, method: string) => App.StartTrain(id, method),
  cancelTrain: (id: string) => App.CancelTrain(id),
  cleanPlan: () => App.CleanPlan() as unknown as Promise<CleanItem[]>,
  clean: (ids: string[]) => App.Clean(ids) as unknown as Promise<CleanResult[]>,
}

export function on<T>(event: string, fn: (data: T) => void): () => void {
  return EventsOn(event, fn as (...data: unknown[]) => void)
}

export function errorText(e: unknown): string {
  return typeof e === 'string' ? e : e instanceof Error ? e.message : String(e)
}
