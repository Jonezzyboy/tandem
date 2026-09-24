import * as App from '../../wailsjs/go/main/App'
import { EventsOn } from '../../wailsjs/runtime/runtime'
import type {
  ChangeSummary, ChangeView, CheckEvent, Inbox, LegResult, PRPreview, PRRequest, RepoInfo, StartItem,
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
}

export function on<T>(event: string, fn: (data: T) => void): () => void {
  return EventsOn(event, fn as (...data: unknown[]) => void)
}

export function errorText(e: unknown): string {
  return typeof e === 'string' ? e : e instanceof Error ? e.message : String(e)
}
