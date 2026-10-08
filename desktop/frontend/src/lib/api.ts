import * as App from '@wailsjs/go/main/App'
import { EventsOn } from '@wailsjs/runtime/runtime'
import type {
  BranchInfo, ChangeSummary, ChangeView, CheckEvent, CleanItem, CleanResult, Inbox, JiraAccount, JiraTicket, LegResult, PinItem, PRPreview, PRRequest,
  TestCheck, TestFinish, TesterPlan, TesterQueue, TesterState, TestRepoStatus, TestStep, VerdictOption,
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
  branchRepos: (branch: string) => App.BranchRepos(branch) as unknown as Promise<BranchInfo[]>,
  start: (id: string, title: string, repos: string[], worktrees = false, ticket = '') =>
    App.Start({ id, title, repos, worktrees, ticket } as never) as unknown as Promise<StartItem[]>,
  link: (id: string, up: string, down: string) => App.Link(id, up, down),
  rename: (id: string, title: string) => App.Rename(id, title),
  reorder: (ids: string[]) => App.Reorder(ids),
  setTicket: (id: string, link: string) => App.SetTicket(id, link),
  jiraLookup: (link: string) => App.JiraLookup(link) as unknown as Promise<JiraTicket>,
  jiraAccount: () => App.JiraAccount() as unknown as Promise<JiraAccount>,
  saveJira: (email: string, token: string) => App.SaveJira(email, token) as unknown as Promise<JiraAccount>,
  testerQueue: (force = false) => App.TesterQueue(force) as unknown as Promise<TesterQueue>,
  testerPlan: (key: string) => App.TesterPlan(key) as unknown as Promise<TesterPlan>,
  testerState: () => App.TesterState() as unknown as Promise<TesterState>,
  testerStart: (req: { key: string; title: string; url: string; repos: { name: string; branch: string }[]; setAside: boolean; checks: TestCheck[] }) =>
    App.TesterStart(req as never) as unknown as Promise<TestStep[]>,
  testerChecks: (checks: TestCheck[]) => App.TesterChecks(checks as never),
  testerStatus: () => App.TesterStatus() as unknown as Promise<TestRepoStatus[]>,
  testerPull: () => App.TesterPull() as unknown as Promise<TestStep[]>,
  testerFinish: () => App.TesterFinish() as unknown as Promise<TestFinish>,
  testerVerdicts: (key: string) => App.TesterVerdicts(key) as unknown as Promise<VerdictOption[]>,
  testerVerdict: (req: { id: string; note: string; withChecks: boolean; files: { name: string; data: string }[] }) =>
    App.TesterVerdict(req as never),
  jiraStatuses: () => App.JiraStatuses() as unknown as Promise<string[]>,
  openURL: (url: string) => App.OpenURL(url),
  openFolder: (path: string) => App.OpenFolder(path),
  openEditor: (path: string) => App.OpenEditor(path),
  addRepos: (id: string, repos: string[]) => App.AddRepos(id, repos) as unknown as Promise<StartItem[]>,
  removeLeg: (id: string, leg: string) => App.RemoveLeg(id, leg),
  unlink: (id: string, up: string, down: string) => App.Unlink(id, up, down),
  switchTo: (id: string, toBase = false) => App.Switch(id, toBase) as unknown as Promise<LegResult[]>,
  pin: (id: string) => App.Pin(id) as unknown as Promise<PinItem[]>,
  commitPins: (id: string) => App.CommitPins(id) as unknown as Promise<LegResult[]>,
  trainPlan: (id: string) => App.TrainPlan(id) as unknown as Promise<TrainPlan>,
  startTrain: (id: string, method: string) => App.StartTrain(id, method),
  cancelTrain: (id: string) => App.CancelTrain(id),
  cleanPlan: () => App.CleanPlan() as unknown as Promise<CleanItem[]>,
  clean: (ids: string[]) => App.Clean(ids) as unknown as Promise<CleanResult[]>,
  discardPlan: (id: string) => App.DiscardPlan(id) as unknown as Promise<CleanItem>,
  discard: (id: string) => App.Discard(id) as unknown as Promise<CleanResult>,
}

export function on<T>(event: string, fn: (data: T) => void): () => void {
  return EventsOn(event, fn as (...data: unknown[]) => void)
}

export function errorText(e: unknown): string {
  return typeof e === 'string' ? e : e instanceof Error ? e.message : String(e)
}
