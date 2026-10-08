import { api, on } from './api'
import { fail } from './state.svelte'
import type { QueueItem, TestCheck, TestFinish, TesterQueue, TesterState, TestRecord, TestRepoStatus, TestStep } from './types'

export const tester = $state({
  queue: null as TesterQueue | null,
  queueLoading: false,
  state: { current: null, history: [] } as TesterState,
  status: [] as TestRepoStatus[],
  statusLoading: false,
  // The change whose repos are being switched, and each repo's progress.
  starting: '',
  steps: {} as Record<string, TestStep>,
  pulling: false,
  finishing: false,
  // The last finished test, shown on the home page until dismissed: the
  // verdict recorded, if any, and how each repo went back.
  finished: null as { key: string; verdict: string; outcome: TestFinish } | null,
  // Kept here so the queue's filters survive leaving the page and coming back.
  filters: { view: 'all' as QueueView, text: '', ci: 'all' as QueueCI | 'all', status: '', repo: '', assignee: '', sort: 'waiting' as QueueSort },
})

export type QueueView = 'all' | 'ready' | 'retests'
export type QueueSort = 'waiting' | 'updated' | 'priority'

// lastTest is the newest test of key on this machine.
export function lastTest(key: string): TestRecord | undefined {
  return tester.state.history.find((r) => r.key === key)
}

// A ticket is a retest when it was last failed here and is back to test again.
export function isRetest(key: string): boolean {
  return lastTest(key)?.result === 'failed'
}

export type QueueCI = 'passing' | 'failing' | 'running' | 'none' | 'loading'

export function queueCI(item: QueueItem): QueueCI {
  if (!item.prsLoaded) return 'loading'
  if (item.prs.length === 0) return 'none'
  if (item.prs.some((p) => p.fail > 0)) return 'failing'
  if (item.prs.some((p) => p.pending > 0)) return 'running'
  return 'passing'
}

export async function loadQueue(force = false) {
  tester.queueLoading = true
  try {
    tester.queue = await api.testerQueue(force)
  } catch (e) {
    fail(e)
  } finally {
    tester.queueLoading = false
  }
}

export async function loadState() {
  try {
    tester.state = await api.testerState()
  } catch (e) {
    fail(e)
  }
}

export async function loadStatus() {
  if (!tester.state.current) {
    tester.status = []
    return
  }
  tester.statusLoading = true
  try {
    tester.status = await api.testerStatus()
  } catch (e) {
    fail(e)
  } finally {
    tester.statusLoading = false
  }
}

export async function startTest(key: string, title: string, url: string, repos: { name: string; branch: string }[], setAside: boolean, checks: TestCheck[]): Promise<TestStep[]> {
  tester.starting = key
  tester.steps = Object.fromEntries(repos.map((r) => [r.name, { repo: r.name, done: false, ok: false, message: 'waiting…', sha: '' }]))
  try {
    const steps = await api.testerStart({ key, title, url, repos, setAside, checks })
    for (const s of steps) tester.steps[s.repo] = s
    await loadState()
    loadStatus()
    return steps
  } catch (e) {
    fail(e)
    return []
  } finally {
    tester.starting = ''
  }
}

export async function pullLatest() {
  tester.pulling = true
  try {
    const steps = await api.testerPull()
    const stuck = steps.filter((s) => !s.ok)
    if (stuck.length) fail(`${stuck.map((s) => s.repo).join(', ')}: ${stuck[0].message}`)
    await loadStatus()
  } catch (e) {
    fail(e)
  } finally {
    tester.pulling = false
  }
}

export async function saveChecks(checks: TestCheck[]) {
  const current = tester.state.current
  if (!current) return
  current.checks = checks
  try {
    await api.testerChecks(checks)
  } catch (e) {
    fail(e)
  }
}

// verdict moves the ticket under test along the chosen transition, commenting
// with the note and checklist and attaching files. It returns what happened,
// or '' when it failed.
export async function verdict(req: { id: string; note: string; withChecks: boolean; files: { name: string; data: string }[] }): Promise<string> {
  try {
    const msg = await api.testerVerdict(req)
    await loadState()
    loadQueue(true)
    return msg
  } catch (e) {
    fail(e)
    return ''
  }
}

// finish puts every repo back on main. verdictMsg is what was recorded on the
// ticket, for the summary. It returns whether every repo made it back.
export async function finish(verdictMsg = ''): Promise<boolean> {
  const current = tester.state.current
  if (!current) return true
  tester.finishing = true
  try {
    const outcome = await api.testerFinish()
    tester.finished = { key: current.key, verdict: verdictMsg, outcome }
    await loadState()
    loadQueue(true)
    if (outcome.done) tester.status = []
    else loadStatus()
    return outcome.done
  } catch (e) {
    fail(e)
    return false
  } finally {
    tester.finishing = false
  }
}

export function initTester() {
  loadState().then(loadStatus)
  on<TestStep>('tester-step', (s) => {
    if (tester.starting) tester.steps[s.repo] = s
  })
  on<TesterQueue>('tester-queue', (q) => { tester.queue = q })
}
