import { api, on } from './api'
import { fail } from './state.svelte'
import { shortRepo } from './format'
import type { BuildEvent, BuildTarget, QueueItem, TestCheck, TestFinish, TesterQueue, TesterState, TestRecord, TestRepoStatus, TestStep } from './types'

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
  // The packages in the repos under test that can be built, and each one's
  // latest build, by buildKey.
  buildTargets: [] as BuildTarget[],
  builds: {} as Record<string, Build>,
  // When each repo under test last moved, so builds from before look stale.
  movedAt: {} as Record<string, number>,
  // What the last pull did, such as "pulled 2 commits into app-charge".
  pullNote: '',
  // Kept here so the queue's filters survive leaving the page and coming back.
  filters: { view: 'all' as QueueView, text: '', status: '', repo: '', assignee: '', sort: 'waiting' as QueueSort },
})

export interface Build {
  running: boolean
  ok: boolean
  error: string
  lines: string[]
  at: number
}

const BUILD_LINES = 400

export const buildKey = (t: { repo: string; dir: string }) => `${t.repo}\u0000${t.dir}`

export async function loadBuilds() {
  if (!tester.state.current) {
    tester.buildTargets = []
    return
  }
  try {
    tester.buildTargets = await api.testerBuilds()
  } catch (e) {
    fail(e)
  }
}

export async function runBuild(t: BuildTarget) {
  tester.builds[buildKey(t)] = { running: true, ok: false, error: '', lines: [], at: Date.now() }
  try {
    await api.testerBuild(t.repo, t.dir)
  } catch (e) {
    tester.builds[buildKey(t)] = { running: false, ok: false, error: String(e), lines: [], at: Date.now() }
  }
}

export function stopBuild(t: BuildTarget) {
  api.testerBuildStop(t.repo, t.dir)
}

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
  tester.pullNote = ''
  tester.steps = Object.fromEntries(repos.map((r) => [r.name, { repo: r.name, done: false, ok: false, message: 'waiting…', sha: '' }]))
  try {
    const steps = await api.testerStart({ key, title, url, repos, setAside, checks })
    for (const s of steps) {
      tester.steps[s.repo] = s
      tester.movedAt[s.repo] = Date.now()
    }
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
    const moved = steps.filter((s) => s.ok && s.message !== 'up to date')
    for (const s of moved) tester.movedAt[s.repo] = Date.now()
    tester.pullNote = moved.length ? `updated ${moved.map((s) => shortRepo(s.repo)).join(', ')}` : steps.every((s) => s.ok) ? 'already up to date' : ''
    const stuck = steps.filter((s) => !s.ok)
    if (stuck.length) fail(stuck.map((s) => `${shortRepo(s.repo)}: ${s.message}`).join('; '))
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
  if (!verdictMsg && current.verdict) verdictMsg = `${current.key} moved to ${current.verdict.to}`
  tester.finishing = true
  try {
    const outcome = await api.testerFinish()
    tester.finished = { key: current.key, verdict: verdictMsg, outcome }
    await loadState()
    loadQueue(true)
    if (outcome.done) Object.assign(tester, { status: [], buildTargets: [], builds: {}, pullNote: '' })
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
  on<BuildEvent>('tester-build', (ev) => {
    const b = (tester.builds[buildKey(ev)] ??= { running: true, ok: false, error: '', lines: [], at: Date.now() })
    if (ev.done) {
      Object.assign(b, { running: false, ok: ev.ok, error: ev.error, at: Date.now() })
      return
    }
    b.lines.push(ev.line)
    if (b.lines.length > BUILD_LINES) b.lines.splice(0, b.lines.length - BUILD_LINES)
  })
}
