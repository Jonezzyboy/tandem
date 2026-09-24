<script lang="ts">
  import { onMount } from 'svelte'
  import { api } from '@lib/api'
  import { fail } from '@lib/state.svelte'
  import type { RepoInfo } from '@lib/types'

  // exclude lists repos already in the change; they are shown but can't be picked.
  let { selected = $bindable([]), exclude = [] }: { selected?: string[]; exclude?: string[] } = $props()

  const shown = 150
  let repos = $state<RepoInfo[]>([])
  let filter = $state('')

  const matches = $derived.by(() => {
    const terms = filter.toLowerCase().split(/\s+/).filter(Boolean)
    return repos.filter((r) => terms.every((t) => r.name.toLowerCase().includes(t)))
  })

  onMount(() => {
    api.repos().then((r) => (repos = r)).catch(fail)
  })

  function toggle(name: string) {
    if (exclude.includes(name)) return
    selected = selected.includes(name) ? selected.filter((s) => s !== name) : [...selected, name]
  }

  function onFilterKey(e: KeyboardEvent) {
    const first = matches.find((m) => !exclude.includes(m.name))
    if (e.key === 'Enter' && first) {
      e.preventDefault()
      toggle(first.name)
      filter = ''
    }
  }
</script>

<div class="picker">
  <input class="input" bind:value={filter} onkeydown={onFilterKey} placeholder="Filter {repos.length} repos · Enter adds the first match" aria-label="Filter repos" />
  <div class="list" role="listbox" aria-multiselectable="true" aria-label="Repos">
    {#each matches.slice(0, shown) as r (r.name)}
      {@const inChange = exclude.includes(r.name)}
      <label class="repo" class:on={selected.includes(r.name)} class:taken={inChange}>
        <input type="checkbox" checked={selected.includes(r.name) || inChange} disabled={inChange} onchange={() => toggle(r.name)} />
        <span class="mono">{r.name}</span>
        {#if inChange}<span class="small muted">already in this change</span>{/if}
      </label>
    {/each}
    {#if matches.length > shown}<div class="small muted more">{matches.length - shown} more · keep typing to narrow</div>{/if}
  </div>
</div>

<style>
  .picker { display: flex; flex-direction: column; gap: 10px; min-height: 0; flex: 1; }
  .small { font-size: 12px; }
  .list { flex: 1; overflow: auto; border: 1px solid var(--line); border-radius: 12px; padding: 6px; }
  .repo { display: flex; gap: 10px; align-items: center; padding: 7px 10px; border-radius: 7px; font-size: 13px; cursor: pointer; }
  .repo:hover { background: var(--panel); }
  .repo.on { background: var(--selected); }
  .repo.taken { cursor: default; opacity: 0.6; }
  .repo.taken:hover { background: transparent; }
  .repo input { accent-color: var(--accent); }
  .repo .muted { margin-left: auto; }
  .more { padding: 8px 10px; }
</style>
