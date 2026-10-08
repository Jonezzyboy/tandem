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
  mergeSha?: string
}

export interface PinView {
  module: string
  upstream: string
  dir: string
  rev: string
  applied: boolean
  error?: string
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
  pins?: PinView[]
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
  ticket: string
  branch: string
  body: string
  reviewers: string[] | null
  legs: LegView[]
  edges: EdgeView[]
  graphError?: string
  blocked: number
  worktrees: boolean
  remote: boolean
  remoteAt: string
  checkedAt: string
}

export interface ChangeSummary {
  id: string
  checkedOut: boolean
  worktrees: boolean
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
  ready: boolean
  forceWithLease: boolean
}

export interface PlanItem {
  repo: string
  name: string
  level: number
  action: 'create' | 'update' | 'skip'
  ready: boolean
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

export interface BranchInfo {
  name: string
  local: boolean
  remote: boolean
  ahead: number
  current: boolean
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

export type Route =
  | { name: 'inbox' } | { name: 'change'; id: string } | { name: 'new' } | { name: 'settings' }
  | { name: 'ready' } | { name: 'test'; id: string } | { name: 'recent' }

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
  phase: 'waiting' | 'merging' | 'merged' | 'pinned' | 'retrying' | 'skipped' | 'done'
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
  warnings: string[]
}

export interface CleanResult {
  id: string
  ok: boolean
  message: string
}

export interface JiraTicket {
  key: string
  url: string
  summary: string
  type: string
  status: string
  priority: string
  assignee: string
  reporter: string
  // RFC 3339, or empty when Jira didn't say.
  updated: string
  description: string
  // Why the issue couldn't be read; key and url are still set.
  error: string
  noAccess: boolean
}

export interface JiraAccount {
  email: string
  hasToken: boolean
}

export interface TestPR {
  repo: string
  branch: string
  number: number
  url: string
  draft: boolean
  author: string
  updated: string
  pass: number
  fail: number
  pending: number
}

export interface QueueItem {
  key: string
  url: string
  summary: string
  status: string
  type: string
  priority: string
  assignee: string
  updated: string
  // When it moved into status, or empty when Jira didn't say.
  statusSince: string
  prs: TestPR[]
  // False until GitHub has been searched for the ticket's PRs.
  prsLoaded: boolean
}

export interface TesterQueue {
  items: QueueItem[]
  // What's missing before the queue can be read; error is any other failure.
  setup: string
  error: string
  statuses: string[]
  // When Jira was read; loading is true while PRs are still being found.
  at: string
  loading: boolean
  prError: string
}

export interface PlanRepo {
  name: string
  // The change's branch in this repo: the key, or a name starting with it.
  branch: string
  cloned: boolean
  current: string
  dirty: number
  pr: TestPR | null
}

export interface TestCommit {
  sha: string
  subject: string
  at: string
}

export interface RepoChanges {
  repo: string
  commits: TestCommit[]
  error: string
}

export interface TesterPlan {
  ticket: JiraTicket
  repos: PlanRepo[]
  cloneRoot: string
  error: string
  // What to try, from the ticket's acceptance criteria.
  checklist: TestCheck[]
  // The previous test of this ticket here, and what was pushed since.
  last: TestRecord | null
  changes: RepoChanges[]
}

export interface TestCheck {
  // The section of the ticket the check came from, if any.
  group?: string
  text: string
  done: boolean
}

export interface TestStep {
  repo: string
  done: boolean
  ok: boolean
  message: string
  sha: string
}

export interface TestSession {
  key: string
  title: string
  url: string
  started: string
  repos: { name: string; dir: string; stash?: string }[]
  checks?: TestCheck[]
}

export interface TestRecord {
  key: string
  title: string
  url: string
  result: 'passed' | 'failed' | 'moved' | 'stopped'
  verdict?: string
  to?: string
  note?: string
  started?: string
  at: string
  checks?: number
  checked?: number
}

export interface TesterState {
  current: TestSession | null
  history: TestRecord[]
}

export interface TestRepoStatus {
  name: string
  branch: string
  onBranch: boolean
  current: string
  behind: number
  new: string[]
  dirty: number
  error: string
}

// A package in a repo under test with a build script; dir is relative to the
// repo, "" for its root.
export interface BuildTarget {
  repo: string
  dir: string
}

export interface BuildEvent {
  repo: string
  dir: string
  line: string
  done: boolean
  ok: boolean
  error: string
}

export interface TestFinish {
  steps: TestStep[]
  done: boolean
}

// One of a ticket's transitions, offered as a test result.
export interface VerdictOption {
  id: string
  name: string
  to: string
  outcome: 'passed' | 'failed' | 'moved'
}
