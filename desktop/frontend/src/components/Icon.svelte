<script lang="ts">
  type Name =
    | 'check' | 'x' | 'clock' | 'running' | 'alert' | 'refresh' | 'sync' | 'play'
    | 'folder' | 'code' | 'external' | 'plus' | 'inbox' | 'branch' | 'send' | 'close'

  let { name, size = 16, color = 'currentColor', spin = false }: { name: Name; size?: number; color?: string; spin?: boolean } = $props()

  // Only round glyphs read as spinning; anything else becomes the busy ring.
  const round: Name[] = ['refresh', 'sync', 'running']
  const shown = $derived(spin && !round.includes(name) ? 'running' : name)
</script>

<svg class="icon" class:spin width={size} height={size} viewBox="0 0 16 16" fill="none" stroke={color} stroke-width="1.6"
  stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
  {#if shown === 'check'}
    <circle cx="8" cy="8" r="6.5" /><path d="M5 8.2l2 2 4-4.4" />
  {:else if shown === 'x'}
    <circle cx="8" cy="8" r="6.5" /><path d="M5.8 5.8l4.4 4.4M10.2 5.8l-4.4 4.4" />
  {:else if shown === 'clock'}
    <circle cx="8" cy="8" r="6.5" /><path d="M8 4.8V8l2.2 1.6" />
  {:else if shown === 'running'}
    <circle cx="8" cy="8" r="6.5" stroke-dasharray="3 2.4" />
  {:else if shown === 'alert'}
    <circle cx="8" cy="8" r="6.5" /><path d="M8 4.6v4.2M8 11.2v.2" />
  {:else if shown === 'refresh'}
    <path d="M13.5 8a5.5 5.5 0 1 1-1.6-3.9" /><path d="M13.5 2.5v3h-3" />
  {:else if shown === 'sync'}
    <path d="M2.5 6.5A5.5 5.5 0 0 1 12.6 4.5M13.5 9.5A5.5 5.5 0 0 1 3.4 11.5" /><path d="M12.8 1.8v2.9H9.9M3.2 14.2v-2.9h2.9" />
  {:else if shown === 'play'}
    <path d="M5 3.5v9l7-4.5z" />
  {:else if shown === 'folder'}
    <path d="M2 4.5A1.5 1.5 0 0 1 3.5 3h2.6l1.5 1.6h4.9A1.5 1.5 0 0 1 14 6.1v5.4a1.5 1.5 0 0 1-1.5 1.5h-9A1.5 1.5 0 0 1 2 11.5z" />
  {:else if shown === 'code'}
    <path d="M5.5 4.5L2 8l3.5 3.5M10.5 4.5L14 8l-3.5 3.5" />
  {:else if shown === 'external'}
    <path d="M9 2.5h4.5V7M13.5 2.5L7.5 8.5M11.5 9.5v3a1 1 0 0 1-1 1h-7a1 1 0 0 1-1-1v-7a1 1 0 0 1 1-1h3" />
  {:else if shown === 'plus'}
    <path d="M8 3v10M3 8h10" />
  {:else if shown === 'inbox'}
    <path d="M2 9l1.8-5.2A1.2 1.2 0 0 1 4.9 3h6.2a1.2 1.2 0 0 1 1.1.8L14 9v3.5a1 1 0 0 1-1 1H3a1 1 0 0 1-1-1z" /><path d="M2 9h3.5l1 1.8h3l1-1.8H14" />
  {:else if shown === 'branch'}
    <circle cx="4.5" cy="3.5" r="1.5" /><circle cx="4.5" cy="12.5" r="1.5" /><circle cx="11.5" cy="5" r="1.5" /><path d="M4.5 5v6M11.5 6.5c0 3-7 2-7 4.5" />
  {:else if shown === 'send'}
    <path d="M14 2L7 9M14 2l-4.5 12L7 9 2 6.5z" />
  {:else if shown === 'close'}
    <path d="M4 4l8 8M12 4l-8 8" />
  {/if}
</svg>
