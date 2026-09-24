<script lang="ts">
  import type { Account } from '@lib/settings.svelte'

  let { account, size = 28 }: { account: Account | null; size?: number } = $props()

  let failed = $state(false)
  const initials = $derived(
    (account?.name || account?.login || '?').split(/\s+/).map((w) => w[0]).slice(0, 2).join('').toUpperCase(),
  )
  // GitHub serves avatars at the requested size; 2x covers Retina.
  const src = $derived(account?.avatarUrl ? `${account.avatarUrl}${account.avatarUrl.includes('?') ? '&' : '?'}s=${size * 2}` : '')
</script>

{#if src && !failed}
  <img {src} alt="" width={size} height={size} style="width: {size}px; height: {size}px" onerror={() => (failed = true)} />
{:else}
  <span class="initials" style="width: {size}px; height: {size}px; font-size: {Math.round(size * 0.38)}px" aria-hidden="true">{initials}</span>
{/if}

<style>
  img, .initials { border-radius: 50%; flex-shrink: 0; }
  img { display: block; background: var(--raised); box-shadow: 0 0 0 1px var(--line); }
  .initials { display: grid; place-items: center; background: var(--accent-bg); color: var(--accent-text); font-weight: 600; }
</style>
