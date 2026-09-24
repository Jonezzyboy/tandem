export interface PRView {
  number: number
  url: string
  state: string
  draft: boolean
  review: string
  pass: number
  fail: number
  pending: number
  failing: string[] | null
}

export interface LegView {
  repo: string
  name: string
  dir: string
  lang: string
  current: string
  onBranch: boolean
  base: string
  baseRef: string
  level: number
  ahead: number
  behind: number
  dirty: number
  localError?: string
  pr: PRView | null
  prError?: string
  blockers: string[]
}

export interface EdgeView {
  from: string
  to: string
  via?: string
  kind?: 'go' | 'composer' | 'npm'
}

export interface ChangeView {
  id: string
  title: string
  branch: string
  body: string
  reviewers: string[] | null
  legs: LegView[]
  edges: EdgeView[]
  graphError?: string
  blocked: number
  remote: boolean
  remoteAt: string
  checkedAt: string
}

export interface ChangeSummary {
  id: string
  checkedOut: boolean
  title: string
  legs: number
  blocked: number
  failing: number
  remote: boolean
  headline: string
  tone: 'ok' | 'warn' | 'muted'
  created: string
}

export interface CheckEvent {
  change: string
  leg: string
  name: string
  state: 'running' | 'pass' | 'fail' | 'skip'
  ms: number
  output: string
}

export interface LegResult {
  leg: string
  ok: boolean
  message: string
}

export interface PRRequest {
  title: string
  body: string
  reviewers: string[]
  draft: boolean
  forceWithLease: boolean
}

export interface PlanItem {
  repo: string
  name: string
  level: number
  action: 'create' | 'update' | 'skip'
  note: string
  error: string
  dirty: number
  url: string
}

export interface PRPreview {
  title: string
  items: PlanItem[]
}

export interface InboxItem {
  repo: string
  number: number
  title: string
  url: string
  author: string
  draft: boolean
  updatedAt: string
  changeId: string
}

export interface Inbox {
  review: InboxItem[] | null
  mine: InboxItem[] | null
  reviewError: string
  mineError: string
  fetchedAt: string
}

export interface RepoInfo {
  name: string
  path: string
}

export interface StartItem {
  repo: string
  ok: boolean
  existing: boolean
  message: string
}

export interface Activity {
  at: number
  text: string
  tone: 'ok' | 'warn' | 'muted'
}

export type Route = { name: 'inbox' } | { name: 'change'; id: string } | { name: 'new' } | { name: 'settings' }

export interface PinItem {
  leg: string
  module: string
  rev: string
  status: 'pinned' | 'already' | 'skipped' | 'failed'
  message: string
}

export interface TrainLeg {
  repo: string
  name: string
  level: number
  pr: number
  url: string
  merged: boolean
  problems: string[]
}

export interface TrainPlan {
  legs: TrainLeg[]
  toMerge: number
  blocked: number
  running: boolean
}

export interface TrainEvent {
  change: string
  leg: string
  level: number
  phase: 'waiting' | 'merging' | 'merged' | 'pinned' | 'skipped' | 'done'
  detail: string
  url: string
}

export interface TrainRun {
  running: boolean
  events: TrainEvent[]
  error: string
}

export interface CleanItem {
  id: string
  title: string
  ready: boolean
  reason: string
  switches: string[]
  worktrees: string[]
  branches: string[]
  kept: string[]
  files: string[]
  dir: string
}

export interface CleanResult {
  id: string
  ok: boolean
  message: string
}
