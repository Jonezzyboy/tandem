<script lang="ts">
  import { api } from '@lib/api'
  import { fail } from '@lib/state.svelte'
  import type { BranchInfo } from '@lib/types'
  import Icon from './Icon.svelte'

  // exclude lists repos already in the change; they are left out of the matches.
  let { branch, selected = $bindable([]), exclude = [] }: { branch: string; selected?: string[]; exclude?: string[] } = $props()

  let found = $state<BranchInfo[]>([])
  let scanning = $state(false)
  let scanned = $state('')
  let seq = 0

  const matches = $derived(found.filter((b) => !exclude.includes(b.name)))
  const unpicked = $derived(matches.filter((b) => !selected.includes(b.name)))

  // Typing an ID rescans every clone, so wait for a pause.
  $effect(() => {
    const b = branch.trim()
    const n = ++seq
    if (!b) {
      found = []
      scanned = ''
      return
    }
    const t = setTimeout(() => {
      scanning = true
      api.branchRepos(b)
        .then((r) => { if (n === seq) { found = r; scanned = b } })
        .catch(fail)
        .finally(() => { if (n === seq) scanning = false })
    }, 300)
    return () => clearTimeout(t)
  })

  function toggle(name: string) {
    selected = selected.includes(name) ? selected.filter((s) => s !== name) : [...selected, name]
  }

  function pickAll() {
    selected = [...selected, ...unpicked.map((b) => b.name)]
  }

  function describe(b: BranchInfo): string {
    const parts = [b.ahead ? `${b.ahead} commit${b.ahead === 1 ? '' : 's'} ahead` : 'no new commits']
    if (!b.local) parts.push('on origin only')
    if (b.current) parts.push('checked out')
    return parts.join(' · ')
  }
</script>

{#if matches.length}
  <div class="existing">
    <div class="head">
      <span class="small"><Icon name="branch" size={13} /> <span class="mono">{scanned}</span> already exists in {matches.length} repo{matches.length === 1 ? '' : 's'}</span>
      {#if unpicked.length}<button class="btn small" onclick={pickAll}>Pick {unpicked.length === matches.length ? 'all' : `the other ${unpicked.length}`}</button>{/if}
    </div>
    {#each matches as b (b.name)}
      <label class="row" class:on={selected.includes(b.name)}>
        <input type="checkbox" checked={selected.includes(b.name)} onchange={() => toggle(b.name)} />
        <span class="mono">{b.name}</span>
        <span class="small muted">{describe(b)}</span>
      </label>
    {/each}
  </div>
{:else if scanning}
  <span class="small muted">Looking for existing <span class="mono">{branch.trim()}</span> branches…</span>
{/if}

<style>
  .small { font-size: 12px; }
  .existing { display: flex; flex-direction: column; gap: 2px; padding: 8px; border: 1px solid var(--line); border-radius: 10px; background: var(--raised); }
  .head { display: flex; justify-content: space-between; align-items: center; gap: 10px; padding: 2px 4px 6px; }
  .head .small { display: inline-flex; align-items: center; gap: 6px; color: var(--text-2); }
  .row { display: flex; gap: 10px; align-items: center; padding: 6px 6px; border-radius: 7px; font-size: 13px; cursor: pointer; }
  .row:hover { background: var(--panel); }
  .row.on { background: var(--selected); }
  .row input { accent-color: var(--accent); }
  .row .muted { margin-left: auto; white-space: nowrap; }
</style>
