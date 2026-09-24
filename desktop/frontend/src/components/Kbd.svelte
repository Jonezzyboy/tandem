<script lang="ts">
  let { keys }: { keys: string[] } = $props()

  const names: Record<string, string> = { '⌘': 'Command', '⌥': 'Option', '⇧': 'Shift', '⌃': 'Control' }
</script>

<kbd aria-label={keys.map((k) => names[k] ?? k).join(' ')}>
  {#each keys as k}<span>{k}</span>{/each}
</kbd>

<style>
  kbd {
    display: inline-flex;
    align-items: center;
    gap: 2px;
    height: 18px;
    padding: 0 5px;
    border: 1px solid var(--line-2);
    border-radius: 5px;
    background: var(--panel);
    color: var(--text-2);
    /* The system face draws ⌘ as Apple's command glyph at the right weight. */
    font: 500 11px/1 -apple-system, system-ui, sans-serif;
    font-variant-numeric: tabular-nums;
  }
  span:first-child:not(:only-child) { font-size: 12px; }
</style>
