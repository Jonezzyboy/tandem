<script lang="ts">
  import { api } from '@lib/api'
  import { continueList, htmlToMarkdown, isListLine, renderMarkdown } from '@lib/markdown'

  let { value = $bindable(), id, placeholder = '' }: { value: string; id: string; placeholder?: string } = $props()

  let mode = $state<'write' | 'preview'>('write')
  let area = $state<HTMLTextAreaElement>()
  const html = $derived(mode === 'preview' ? renderMarkdown(value) : '')

  // execCommand keeps the edit on the textarea's undo stack.
  function insert(text: string) {
    if (!document.execCommand('insertText', false, text)) {
      area!.setRangeText(text, area!.selectionStart, area!.selectionEnd, 'end')
      value = area!.value
    }
  }

  function onPaste(e: ClipboardEvent) {
    const md = htmlToMarkdown(e.clipboardData?.getData('text/html') ?? '')
    if (md === null) return
    e.preventDefault()
    insert(md)
  }

  function lineBounds(a: HTMLTextAreaElement) {
    const start = a.value.lastIndexOf('\n', a.selectionStart - 1) + 1
    const to = a.selectionEnd > a.selectionStart && a.value[a.selectionEnd - 1] === '\n' ? a.selectionEnd - 1 : a.selectionEnd
    const nl = a.value.indexOf('\n', to)
    return { start, end: nl < 0 ? a.value.length : nl }
  }

  function onKey(e: KeyboardEvent) {
    const a = area!
    if (e.isComposing || e.metaKey || e.ctrlKey || e.altKey) return
    if (e.key === 'Enter' && !e.shiftKey && a.selectionStart === a.selectionEnd) {
      const { start } = lineBounds(a)
      const next = continueList(a.value.slice(start, a.selectionStart))
      if (!next) return
      e.preventDefault()
      if ('insert' in next) {
        insert(next.insert)
      } else {
        a.setSelectionRange(start, start + next.end)
        insert('')
      }
      return
    }
    if (e.key === 'Tab') {
      const { start, end } = lineBounds(a)
      const lines = a.value.slice(start, end).split('\n')
      if (lines.length === 1 && !isListLine(lines[0])) return
      e.preventDefault()
      const [selStart, selEnd] = [a.selectionStart, a.selectionEnd]
      const shifted = lines.map((l) => (e.shiftKey ? l.replace(/^ {1,2}/, '') : '  ' + l))
      const firstDelta = shifted[0].length - lines[0].length
      const totalDelta = shifted.join('\n').length - (end - start)
      a.setSelectionRange(start, end)
      insert(shifted.join('\n'))
      a.setSelectionRange(Math.max(start, selStart + firstDelta), Math.max(start, selEnd + totalDelta))
    }
  }

  function onPreviewClick(e: MouseEvent) {
    const link = (e.target as HTMLElement).closest('a')
    if (!link) return
    e.preventDefault()
    api.openURL(link.href)
  }
</script>

<div class="editor">
  <div class="tabs" role="tablist">
    <button role="tab" class:on={mode === 'write'} aria-selected={mode === 'write'} onclick={() => (mode = 'write')}>Write</button>
    <button role="tab" class:on={mode === 'preview'} aria-selected={mode === 'preview'} onclick={() => (mode = 'preview')}>Preview</button>
    <span class="muted hint">Markdown · rich text pastes as Markdown</span>
  </div>
  {#if mode === 'write'}
    <textarea {id} class="input" bind:this={area} bind:value {placeholder} onpaste={onPaste} onkeydown={onKey} spellcheck="true"></textarea>
  {:else}
    <!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
    <div class="preview selectable" onclick={onPreviewClick}>
      {#if value.trim()}{@html html}{:else}<p class="muted">Nothing to preview.</p>{/if}
    </div>
  {/if}
</div>

<style>
  .editor { display: flex; flex-direction: column; flex: 1; min-height: 0; gap: 8px; }
  .tabs { display: flex; align-items: center; gap: 4px; }
  .tabs button {
    background: none; border: 1px solid transparent; border-radius: 6px; padding: 3px 10px;
    font-size: 12.5px; color: var(--muted); cursor: pointer;
  }
  .tabs button:hover { color: var(--text); }
  .tabs button.on { color: var(--text); background: var(--raised); border-color: var(--line); }
  .hint { margin-left: auto; font-size: 11.5px; }
  textarea { flex: 1; min-height: 0; font-size: 13.5px; tab-size: 2; }
  .preview {
    flex: 1; min-height: 0; overflow: auto; padding: 10px 14px; border: 1px solid var(--line); border-radius: 8px;
    background: var(--raised); font-size: 13.5px; line-height: 1.55;
  }
  .preview :global(:first-child) { margin-top: 0; }
  .preview :global(p), .preview :global(ul), .preview :global(ol), .preview :global(pre), .preview :global(blockquote), .preview :global(table) { margin: 0 0 0.8em; }
  .preview :global(ul), .preview :global(ol) { padding-left: 1.6em; }
  .preview :global(li > ul), .preview :global(li > ol) { margin: 0.2em 0 0; }
  .preview :global(h1), .preview :global(h2), .preview :global(h3) { margin: 1em 0 0.5em; font-size: 1.1em; }
  .preview :global(code) { font-family: var(--mono); font-size: 0.9em; padding: 1px 5px; border-radius: 4px; background: var(--hover); }
  .preview :global(pre) { padding: 10px 12px; border-radius: 8px; background: var(--bg); border: 1px solid var(--line); overflow-x: auto; }
  .preview :global(pre code) { padding: 0; background: none; }
  .preview :global(blockquote) { padding-left: 12px; border-left: 3px solid var(--line-2); color: var(--text-2); }
  .preview :global(table) { border-collapse: collapse; }
  .preview :global(th), .preview :global(td) { padding: 4px 10px; border: 1px solid var(--line-2); text-align: left; }
  .preview :global(a) { color: var(--accent-text); }
</style>
