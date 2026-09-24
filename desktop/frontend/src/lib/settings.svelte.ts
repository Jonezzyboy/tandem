import * as App from '@wailsjs/go/main/App'
import { errorText } from './api'

export type ThemeId = 'system' | 'graphite' | 'paper' | 'midnight' | 'forest' | 'contrast'

export interface Settings {
  theme: ThemeId
  keys: Record<string, string>
  editor: string
  mergeMethod: 'squash' | 'merge' | 'rebase'
  draftPRs: boolean
}

export interface Account {
  login: string
  name: string
  avatarUrl: string
  error: string
}

export const themes: { id: ThemeId; name: string; note: string }[] = [
  { id: 'graphite', name: 'Graphite', note: 'The default dark theme' },
  { id: 'paper', name: 'Paper', note: 'Light, on a warm ground' },
  { id: 'midnight', name: 'Midnight', note: 'Deep blue dark' },
  { id: 'forest', name: 'Forest', note: 'Green-tinted dark' },
  { id: 'contrast', name: 'High contrast', note: 'Pure black, brighter text and borders' },
  { id: 'system', name: 'System', note: 'Paper or Graphite, following macOS' },
]

export type Scope = 'app' | 'change'

export interface Action {
  id: string
  label: string
  scope: Scope
  default: string
}

// Shortcuts are "meta+shift+s": modifiers in this order, then the key as
// KeyboardEvent.key lower-cased.
export const actions: Action[] = [
  { id: 'newChange', label: 'New change', scope: 'app', default: 'meta+n' },
  { id: 'inbox', label: 'Go to Inbox', scope: 'app', default: 'meta+0' },
  { id: 'settings', label: 'Open Settings', scope: 'app', default: 'meta+,' },
  { id: 'refresh', label: 'Refresh from GitHub', scope: 'app', default: 'meta+r' },
  { id: 'syncAll', label: 'Sync all legs', scope: 'change', default: 'meta+shift+s' },
  { id: 'runChecks', label: 'Run checks', scope: 'change', default: 'meta+shift+k' },
  { id: 'publishPRs', label: 'Open or publish PRs', scope: 'change', default: 'meta+shift+p' },
  { id: 'mergeTrain', label: 'Open the merge train', scope: 'change', default: 'meta+shift+m' },
]

// Owned by macOS or the app's jump-to-change keys; never assignable.
const reserved = new Set([
  'meta+q', 'meta+w', 'meta+h', 'meta+m', 'meta+c', 'meta+v', 'meta+x', 'meta+a', 'meta+z',
  'meta+shift+z', 'meta+tab', 'meta+ ', 'meta+alt+h',
  ...Array.from({ length: 9 }, (_, i) => `meta+${i + 1}`),
])

const modifierKeys = new Set(['meta', 'control', 'alt', 'shift', 'capslock', 'fn'])

export const prefs = $state({
  settings: {
    theme: 'graphite', keys: {}, editor: '', mergeMethod: 'squash', draftPRs: true,
  } as Settings,
  account: null as Account | null,
  systemDark: true,
  version: '',
  path: '',
  home: '',
  error: '',
  // True while Settings records a shortcut, so App's handler stands aside.
  recording: false,
})

export function shortcutFromEvent(e: KeyboardEvent): string | null {
  const key = e.key.toLowerCase()
  if (modifierKeys.has(key)) return null
  const mods = [e.metaKey && 'meta', e.ctrlKey && 'ctrl', e.altKey && 'alt', e.shiftKey && 'shift'].filter(Boolean)
  // With Option held, e.key is the composed character; the physical key is steadier.
  const base = e.altKey && /^Key[A-Z]$|^Digit\d$/.test(e.code) ? e.code.slice(-1).toLowerCase() : key
  return [...mods, base].join('+')
}

export function shortcutFor(id: string): string {
  return prefs.settings.keys[id] ?? actions.find((a) => a.id === id)?.default ?? ''
}

const glyphs: Record<string, string> = { meta: '⌘', ctrl: '⌃', alt: '⌥', shift: '⇧' }
const keyNames: Record<string, string> = {
  arrowup: '↑', arrowdown: '↓', arrowleft: '←', arrowright: '→', enter: '↩', escape: 'Esc',
  backspace: '⌫', delete: '⌦', tab: '⇥', ' ': 'Space',
}

export function keycaps(shortcut: string): string[] {
  if (!shortcut) return []
  const parts = shortcut.split('+')
  // "meta++" is a literal plus key.
  const key = shortcut.endsWith('++') ? '+' : parts[parts.length - 1]
  const mods = parts.slice(0, shortcut.endsWith('++') ? -2 : -1)
  return [...mods.map((m) => glyphs[m] ?? m), keyNames[key] ?? key.toUpperCase()]
}

// problem explains why shortcut cannot be given to action id, or is "".
export function problem(id: string, shortcut: string): string {
  const parts = shortcut.split('+')
  if (!parts.some((p) => p === 'meta' || p === 'ctrl' || p === 'alt')) {
    return 'Include ⌘, ⌃ or ⌥ so typing never triggers it'
  }
  if (reserved.has(shortcut)) return 'Reserved by macOS or for jumping to a change'
  const taken = actions.find((a) => a.id !== id && shortcutFor(a.id) === shortcut)
  return taken ? `Already used by “${taken.label}”` : ''
}

export function resolvedTheme(): Exclude<ThemeId, 'system'> {
  const t = prefs.settings.theme
  if (t !== 'system') return t
  return prefs.systemDark ? 'graphite' : 'paper'
}

export function applyTheme() {
  document.documentElement.dataset.theme = resolvedTheme()
}

export async function saveSettings(next: Partial<Settings>) {
  const merged = { ...prefs.settings, ...next }
  prefs.settings = merged
  applyTheme()
  try {
    prefs.settings = (await App.SaveSettings(merged as never)) as unknown as Settings
    prefs.error = ''
  } catch (e) {
    prefs.error = errorText(e)
  }
}

export async function refreshSystemAppearance() {
  if (prefs.settings.theme !== 'system') return
  prefs.systemDark = await App.SystemDark()
  applyTheme()
}

export async function initSettings() {
  const [settings, systemDark] = await Promise.all([App.Settings(), App.SystemDark()])
  prefs.settings = settings as unknown as Settings
  prefs.systemDark = systemDark
  applyTheme()
  App.Account().then((a) => { prefs.account = a as unknown as Account })
  App.Version().then((v) => { prefs.version = v })
  App.SettingsPath().then((p) => { prefs.path = p })
  App.TandemHome().then((h) => { prefs.home = h })
  window.addEventListener('focus', refreshSystemAppearance)
}
