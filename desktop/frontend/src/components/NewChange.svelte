<script lang="ts">
  import { api } from '@lib/api'
  import { fail, navigate } from '@lib/state.svelte'
  import { prefs } from '@lib/settings.svelte'
  import type { StartItem } from '@lib/types'
  import Icon from './Icon.svelte'
  import RepoPicker from './RepoPicker.svelte'

  let id = $state('')
  let title = $state('')
  let selected = $state<string[]>([])
  let worktrees = $state(false)
  let starting = $state(false)
  let results = $state<StartItem[] | null>(null)

  const validId = $derived(/^[A-Za-z0-9][A-Za-z0-9._-]*$/.test(id) && !id.includes('..'))

  function unpick(name: string) {
    selected = selected.filter((s) => s !== name)
  }

  async function start() {
    starting = true
    try {
      results = await api.start(id, title, selected, worktrees)
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
    <div class="muted mono small">{worktrees ? 'One worktree per repo, beside your clones' : 'One branch per repo, checked out in your clones'}</div>
    <h1>New change</h1>
  </header>

  <div class="grid">
    <div class="form">
      <div class="field">
        <label for="nc-id">Ticket or change ID</label>
        <input id="nc-id" class="input mono" bind:value={id} placeholder="ABC-123" autocomplete="off" />
        <span class="small muted">{worktrees ? 'The branch name in every repo’s worktree.' : 'The branch name in every repo. Clean repos switch to it; any with uncommitted work stay put.'}</span>
      </div>
      <div class="field">
        <label for="nc-title">Title</label>
        <input id="nc-title" class="input" bind:value={title} placeholder="What this change does" />
      </div>
      <div class="field">
        <span class="label">Repos · {selected.length} selected</span>
        <div class="chips">
          {#each selected as s (s)}
            <button class="chip mono" onclick={() => unpick(s)} aria-label="Remove {s}">{s} <Icon name="close" size={12} /></button>
          {:else}
            <span class="small muted">Pick repos from the list →</span>
          {/each}
        </div>
      </div>
      <label class="mode">
        <input type="checkbox" bind:checked={worktrees} />
        <span class="stack">
          <span>Use worktrees instead</span>
          <span class="small muted">A worktree per repo under <span class="mono">{prefs.home || '~/code/.tandem'}/{id || 'ID'}</span>, so your clones and anything running from them stay on their current branches. Repos added later follow this.</span>
        </span>
      </label>
      <button class="btn primary start" disabled={!validId || selected.length === 0 || starting} onclick={start}>
        <Icon name="branch" spin={starting} />
        Create {selected.length || ''} {worktrees ? `worktree${selected.length === 1 ? '' : 's'}` : `branch${selected.length === 1 ? '' : 'es'}`}
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

    <RepoPicker bind:selected />
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
  .mode { display: flex; gap: 10px; align-items: flex-start; cursor: pointer; font-size: 14px; }
  .mode input { width: 16px; height: 16px; margin-top: 2px; accent-color: var(--accent); }
  .mode .stack { display: flex; flex-direction: column; gap: 3px; line-height: 1.4; }
  .mode .mono { white-space: nowrap; }
  .results { display: flex; flex-direction: column; gap: 8px; }
  .result { display: grid; grid-template-columns: 16px auto; gap: 4px 10px; align-items: center; font-size: 13px; }
  .result span:last-child { grid-column: 2; word-break: break-all; }
</style>
