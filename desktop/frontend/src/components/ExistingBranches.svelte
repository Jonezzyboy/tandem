<script lang="ts">
  import { api } from '@lib/api'
  import { fail } from '@lib/state.svelte'
  import type { BranchInfo } from '@lib/types'
  import Icon from './Icon.svelte'

  // exclude lists repos already in the change; they are left out of the matches.
  let { branch, selected = $bindable([]), existing = $bindable([]), exclude = [] }: {
    branch: string; selected?: string[]; existing?: string[]; exclude?: string[]
  } = $props()

  let found = $state<BranchInfo[]>([])
  let scanning = $state(false)
  let scanned = $state('')
  let seq = 0

  const matches = $derived(found.filter((b) => !exclude.includes(b.name)))
  const unpicked = $derived(matches.filter((b) => !selected.includes(b.name)))

  $effect(() => { existing = matches.map((b) => b.name) })

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
      <Icon name="branch" color="var(--accent-text)" />
      <div class="stack">
        <strong>Existing branch found</strong>
        <span class="small"><span class="mono">{scanned}</span> is already in {matches.length} repo{matches.length === 1 ? '' : 's'}. Pick {matches.length === 1 ? 'it' : 'them'} to load with {matches.length === 1 ? 'its' : 'their'} commits.</span>
      </div>
      {#if unpicked.length}<button class="btn small" onclick={pickAll}>Load {unpicked.length === matches.length ? (matches.length === 1 ? 'it' : 'all') : `the other ${unpicked.length}`}</button>{/if}
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
  .existing { display: flex; flex-direction: column; gap: 2px; padding: 10px; border: 1px solid var(--accent); border-radius: 10px; background: var(--accent-bg); }
  .head { display: flex; align-items: flex-start; gap: 10px; padding: 2px 4px 8px; }
  .head .stack { display: flex; flex-direction: column; gap: 2px; flex: 1; line-height: 1.4; font-size: 13.5px; }
  .head strong { color: var(--accent-text); }
  .head .small { color: var(--text-2); }
  .row { display: flex; gap: 10px; align-items: center; padding: 6px 6px; border-radius: 7px; font-size: 13px; cursor: pointer; }
  .row:hover { background: var(--panel); }
  .row.on { background: var(--selected); }
  .row input { accent-color: var(--accent); }
  .row .muted { margin-left: auto; white-space: nowrap; }
</style>
