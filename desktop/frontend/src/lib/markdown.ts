import { Marked, type Tokens } from 'marked'

const escapeHTML = (s: string) => s.replace(/[&<>"']/g, (c) => `&#${c.charCodeAt(0)};`)
const safeHref = (href: string) => /^(https?:|mailto:)/i.test(href)

// Mirrors GitHub's PR body rendering; raw HTML is shown as text because the preview runs inside the app's webview.
const md = new Marked({
  gfm: true,
  breaks: true,
  renderer: {
    html({ text }: Tokens.HTML | Tokens.Tag) {
      return escapeHTML(text)
    },
    link({ href, tokens }: Tokens.Link) {
      const inner = this.parser.parseInline(tokens)
      return safeHref(href) ? `<a href="${escapeHTML(href)}">${inner}</a>` : inner
    },
    image({ text }: Tokens.Image) {
      return escapeHTML(text)
    },
  },
})

export function renderMarkdown(src: string): string {
  return md.parse(src, { async: false })
}

// Rich clipboard HTML worth converting; code editors paste plain divs and spans, which stay as plain text.
const structured = /<(ul|ol|li|h[1-6]|p|strong|b|em|a|blockquote|table|pre|code)[\s>]/i

export function htmlToMarkdown(html: string): string | null {
  if (!structured.test(html)) return null
  const doc = new DOMParser().parseFromString(html, 'text/html')
  const out = blocks(doc.body)
  return out
    .replace(/[ \t]+$/gm, '')
    .replace(/\n{3,}/g, '\n\n')
    // Google Docs puts each list item in its own list; rejoin them so the list stays tight.
    .replace(/^(\s*(?:[-*+]|\d+[.)]) .*)\n\n(?=\s*(?:[-*+]|\d+[.)]) )/gm, '$1\n')
    .replace(/^\n+|\s+$/g, '')
}

const skip = new Set(['SCRIPT', 'STYLE', 'HEAD', 'META', 'TITLE'])

function blocks(el: Node): string {
  return [...el.childNodes].map(node).join('')
}

function node(n: Node): string {
  if (n.nodeType === Node.TEXT_NODE) return (n.textContent ?? '').replace(/\s+/g, ' ')
  if (n.nodeType !== Node.ELEMENT_NODE) return ''
  const el = n as HTMLElement
  const tag = el.tagName
  if (skip.has(tag)) return ''
  const inner = () => blocks(el)
  const block = (s: string) => `\n\n${s.trim()}\n\n`
  const wrap = (mark: string) => {
    const s = inner()
    return s.trim() ? s.replace(/^(\s*)([\s\S]*?)(\s*)$/, `$1${mark}$2${mark}$3`) : s
  }
  switch (tag) {
    case 'BR':
      return '\n'
    case 'P':
    case 'DIV':
    case 'SECTION':
    case 'ARTICLE':
      return block(inner())
    case 'H1': case 'H2': case 'H3': case 'H4': case 'H5': case 'H6':
      return block(`${'#'.repeat(Number(tag[1]))} ${inner().replace(/\s+/g, ' ')}`)
    case 'B':
    case 'STRONG':
      // Google Docs wraps the whole clipboard in a non-bold <b>.
      return el.style.fontWeight === 'normal' || el.id.startsWith('docs-internal') ? inner() : wrap('**')
    case 'EM':
    case 'I':
      return wrap('_')
    case 'SPAN': {
      const w = el.style.fontWeight
      return w === 'bold' || Number(w) >= 600 ? wrap('**') : el.style.fontStyle === 'italic' ? wrap('_') : inner()
    }
    case 'CODE':
      return '`' + (el.textContent ?? '') + '`'
    case 'PRE':
      return block('```\n' + (el.textContent ?? '').replace(/\n$/, '') + '\n```')
    case 'A': {
      const href = el.getAttribute('href') ?? ''
      const text = inner().trim()
      if (!safeHref(href)) return inner()
      return !text || text === href ? href : `[${text}](${href})`
    }
    case 'BLOCKQUOTE':
      return block(blocks(el).trim().split('\n').map((l) => (l ? `> ${l}` : '>')).join('\n'))
    case 'UL':
    case 'OL':
      return `\n\n${list(el)}\n\n`
    case 'TABLE':
      return block(table(el))
    default:
      return inner()
  }
}

function list(el: HTMLElement): string {
  const ordered = el.tagName === 'OL'
  let i = Number(el.getAttribute('start') ?? 1)
  return [...el.children]
    .filter((c) => c.tagName === 'LI')
    .map((li) => {
      const marker = ordered ? `${i++}. ` : '- '
      // Google Docs flattens nesting into sibling lists and records depth here.
      const depth = Math.max(0, Number(li.getAttribute('aria-level') ?? 1) - 1)
      const pad = '  '.repeat(depth)
      const body = blocks(li).replace(/\n{2,}/g, '\n').trim()
      return body
        .split('\n')
        .map((l, j) => (j === 0 ? pad + marker + l.trim() : l ? pad + ' '.repeat(marker.length) + l : l))
        .join('\n')
    })
    .join('\n')
}

function table(el: HTMLElement): string {
  const rows = [...el.querySelectorAll('tr')].map((tr) =>
    [...tr.children].map((c) => blocks(c).replace(/\s+/g, ' ').trim().replace(/\|/g, '\\|')),
  )
  if (!rows.length) return ''
  const width = Math.max(...rows.map((r) => r.length))
  const line = (r: string[]) => `| ${Array.from({ length: width }, (_, i) => r[i] ?? '').join(' | ')} |`
  return [line(rows[0]), line(Array(width).fill('---')), ...rows.slice(1).map(line)].join('\n')
}

const listItem = /^(\s*)([-*+]|\d+[.)])(\s+)(\[[ xX]\]\s+)?/

// Text to insert for Enter at the end of `line`: the next list marker, or null to end the list.
export function continueList(line: string): { insert: string } | { end: number } | null {
  const m = line.match(listItem)
  if (!m) return null
  if (!line.slice(m[0].length).trim()) return { end: m[0].length }
  const [, indent, marker, gap, box] = m
  const num = marker.match(/^(\d+)(.)$/)
  const next = num ? `${Number(num[1]) + 1}${num[2]}` : marker
  return { insert: `\n${indent}${next}${gap}${box ? '[ ] ' : ''}` }
}

export const isListLine = (line: string) => listItem.test(line)
