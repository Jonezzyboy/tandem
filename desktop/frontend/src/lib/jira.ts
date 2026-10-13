// Keep in sync with jira.Parse in internal/jira.
const key = /^[A-Z][A-Z0-9_]+-[0-9]+$/

// parseTicket finds the issue in a Jira link: a /browse/KEY page, or a board
// or search page with ?selectedIssue=KEY.
export function parseTicket(text: string): { key: string; url: string } | null {
  let u: URL
  try {
    u = new URL(text.trim())
  } catch {
    return null
  }
  if ((u.protocol !== 'https:' && u.protocol !== 'http:') || !u.host) return null
  const parts = u.pathname.split('/').filter(Boolean)
  let k = u.searchParams.get('selectedIssue') ?? ''
  if (!k && parts.length >= 2 && parts[parts.length - 2] === 'browse') k = parts[parts.length - 1]
  k = k.toUpperCase()
  return key.test(k) ? { key: k, url: `https://${u.host}/browse/${k}` } : null
}

// Keep in sync with jira.KeyInBranch in internal/jira.
const branchKey = /(?:^|\/)([A-Za-z][A-Za-z0-9_]+-[0-9]+)(?:$|[/._-])/

// keyInBranch finds an issue key leading a part of a branch name, as in
// DEV-123, feature/DEV-123 or dev-123-login.
export function keyInBranch(branch: string): string {
  return branch.trim().match(branchKey)?.[1].toUpperCase() ?? ''
}
