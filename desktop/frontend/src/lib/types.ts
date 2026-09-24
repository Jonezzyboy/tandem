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
  worktree: string
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

export type Route = { name: 'inbox' } | { name: 'change'; id: string } | { name: 'new' }
