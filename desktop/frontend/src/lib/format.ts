export function ago(iso: string | number, now = Date.now()): string {
  const t = typeof iso === 'number' ? iso : Date.parse(iso)
  if (!t || t < 0) return ''
  const s = Math.max(0, Math.round((now - t) / 1000))
  if (s < 60) return `${s}s`
  const m = Math.round(s / 60)
  if (m < 60) return `${m}m`
  const h = Math.round(m / 60)
  if (h < 48) return `${h}h`
  return `${Math.round(h / 24)}d`
}

export function reviewLabel(review: string): string {
  switch (review) {
    case 'APPROVED':
      return 'Approved'
    case 'CHANGES_REQUESTED':
      return 'Changes requested'
    case 'REVIEW_REQUIRED':
      return 'Awaiting review'
    default:
      return 'No review yet'
  }
}

export function shortRepo(repo: string): string {
  return repo.slice(repo.lastIndexOf('/') + 1)
}

// slackMessage is the note posted in Slack once a change's PRs are open: the
// ticket and title, then each repo's PR in merge order.
export function slackMessage(key: string, title: string, prs: { repo: string; url: string; level: number }[]): string {
  const sorted = prs.filter((p) => p.url).sort((a, b) => a.level - b.level)
  if (!sorted.length) return ''
  return [`${key}${title ? ` - ${title}` : ''}:`, ...sorted.map((p) => `${shortRepo(p.repo)}: ${p.url}`)].join('\n')
}
