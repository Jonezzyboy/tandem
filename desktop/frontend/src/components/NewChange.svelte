<script lang="ts">
  import { onMount } from 'svelte'
  import { api } from '../lib/api'
  import { fail, navigate } from '../lib/state.svelte'
  import type { RepoInfo, StartItem } from '../lib/types'
  import Icon from './Icon.svelte'

  const shown = 150

  let repos = $state<RepoInfo[]>([])
  let id = $state('')
  let title = $state('')
  let filter = $state('')
  let selected = $state<string[]>([])
  let starting = $state(false)
  let results = $state<StartItem[] | null>(null)

  const matches = $derived.by(() => {
    const terms = filter.toLowerCase().split(/\s+/).filter(Boolean)
    return repos.filter((r) => terms.every((t) => r.name.toLowerCase().includes(t)))
  })
  const validId = $derived(/^[A-Za-z0-9][A-Za-z0-9._-]*$/.test(id) && !id.includes('..'))

  onMount(() => {
    api.repos().then((r) => (repos = r)).catch(fail)
  })

  function toggle(name: string) {
    selected = selected.includes(name) ? selected.filter((s) => s !== name) : [...selected, name]
  }

  function onFilterKey(e: KeyboardEvent) {
    if (e.key === 'Enter' && matches.length > 0) {
      e.preventDefault()
      toggle(matches[0].name)
      filter = ''
    }
  }

  async function start() {
    starting = true
    try {
      results = await api.start(id, title, selected)
      if (results.every((r) => r.ok)) navigate({ name: 'change', id })
    } catch (e) {
      fail(e)
    } finally {
      starting = false
    }
  }
</script>

<div class="page">
  <header style="--wails-draggable: drag">
    <div class="muted mono small">One branch, one worktree per repo</div>
    <h1>New change</h1>
  </header>

  <div class="grid">
    <div class="form">
      <div class="field">
        <label for="nc-id">Ticket or change ID</label>
        <input id="nc-id" class="input mono" bind:value={id} placeholder="ABC-123" autocomplete="off" />
        <span class="small muted">Also the branch name in every repo.</span>
      </div>
      <div class="field">
        <label for="nc-title">Title</label>
        <input id="nc-title" class="input" bind:value={title} placeholder="What this change does" />
      </div>
      <div class="field">
        <span class="label">Repos · {selected.length} selected</span>
        <div class="chips">
          {#each selected as s (s)}
            <button class="chip mono" onclick={() => toggle(s)} aria-label="Remove {s}">{s} <Icon name="close" size={12} /></button>
          {:else}
            <span class="small muted">Pick repos from the list →</span>
          {/each}
        </div>
      </div>
      <button class="btn primary start" disabled={!validId || selected.length === 0 || starting} onclick={start}>
        <span class:spin={starting}><Icon name="branch" /></span>
        Create {selected.length || ''} worktree{selected.length === 1 ? '' : 's'}
      </button>
      {#if results}
        <div class="results">
          {#each results as r (r.repo)}
            <div class="result">
              {#if r.ok}<Icon name="check" color="var(--ok)" />{:else}<Icon name="x" color="var(--warn)" />{/if}
              <span class="mono">{r.repo}</span>
              <span class="small selectable" class:warn={!r.ok} class:muted={r.ok}>{r.message}</span>
            </div>
          {/each}
          {#if results.some((r) => r.ok)}
            <button class="btn small" onclick={() => navigate({ name: 'change', id })}>Open {id} →</button>
          {/if}
        </div>
      {/if}
    </div>

    <div class="picker">
      <input class="input" bind:value={filter} onkeydown={onFilterKey} placeholder="Filter {repos.length} repos · Enter adds the first match" aria-label="Filter repos" />
      <div class="list" role="listbox" aria-multiselectable="true" aria-label="Repos">
        {#each matches.slice(0, shown) as r (r.name)}
          <label class="repo" class:on={selected.includes(r.name)}>
            <input type="checkbox" checked={selected.includes(r.name)} onchange={() => toggle(r.name)} />
            <span class="mono">{r.name}</span>
          </label>
        {/each}
        {#if matches.length > shown}<div class="small muted more">{matches.length - shown} more · keep typing to narrow</div>{/if}
      </div>
    </div>
  </div>
</div>

<style>
  .page { padding: 0 32px 32px; display: flex; flex-direction: column; gap: 24px; height: 100%; }
  header { padding-top: 28px; }
  h1 { margin: 4px 0 0; font-family: var(--display); font-weight: 700; font-size: 30px; }
  .small { font-size: 12px; }
  .grid { display: grid; grid-template-columns: 420px minmax(0, 1fr); gap: 20px; flex: 1; min-height: 0; }
  .form { display: flex; flex-direction: column; gap: 18px; padding: 20px; background: var(--panel); border: 1px solid var(--line); border-radius: 12px; align-self: start; }
  .label { font-size: 13px; color: var(--text-2); }
  .chips { display: flex; flex-wrap: wrap; gap: 6px; }
  .chip { display: inline-flex; align-items: center; gap: 6px; padding: 5px 9px; border-radius: 7px; border: 1px solid var(--line-2); background: var(--raised); font-size: 12.5px; cursor: pointer; }
  .start { justify-content: center; min-height: 42px; }
  .results { display: flex; flex-direction: column; gap: 8px; }
  .result { display: grid; grid-template-columns: 16px auto; gap: 4px 10px; align-items: center; font-size: 13px; }
  .result span:last-child { grid-column: 2; word-break: break-all; }
  .picker { display: flex; flex-direction: column; gap: 10px; min-height: 0; }
  .list { flex: 1; overflow: auto; border: 1px solid var(--line); border-radius: 12px; padding: 6px; }
  .repo { display: flex; gap: 10px; align-items: center; padding: 7px 10px; border-radius: 7px; font-size: 13px; cursor: pointer; }
  .repo:hover { background: var(--panel); }
  .repo.on { background: var(--selected); }
  .repo input { accent-color: var(--accent); }
  .more { padding: 8px 10px; }
</style>
