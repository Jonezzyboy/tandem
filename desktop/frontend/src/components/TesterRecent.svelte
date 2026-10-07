<script lang="ts">
  import { onMount } from 'svelte'
  import { api } from '@lib/api'
  import { ago } from '@lib/format'
  import { navigate } from '@lib/state.svelte'
  import { loadState, tester } from '@lib/tester.svelte'

  onMount(loadState)
</script>

<div class="page">
  <header style="--wails-draggable: drag">
    <div class="mono muted small">The last {tester.state.history.length || 'few'} changes you tested on this machine</div>
    <h1>Recently tested</h1>
  </header>
  <section class="list" aria-label="Recently tested">
    {#each tester.state.history as r, i (r.key + r.at + i)}
      <div class="item">
        <span class="result mono {r.result}">{r.result}</span>
        <div class="stack">
          <span>
            {#if r.url}<button class="link mono" onclick={() => api.openURL(r.url)}>{r.key}</button>{:else}<span class="mono">{r.key}</span>{/if}
            <span class="sub">{r.title}</span>
          </span>
          <span class="small muted">{ago(r.at)} ago</span>
        </div>
        <button class="btn small" onclick={() => navigate({ name: 'test', id: r.key })}>Open</button>
      </div>
    {:else}
      <p class="muted">Changes you test show up here once you go back to main.</p>
    {/each}
  </section>
</div>

<style>
  .page { padding: 0 32px 32px; display: flex; flex-direction: column; gap: 18px; }
  header { padding-top: 28px; display: flex; flex-direction: column; gap: 4px; }
  h1 { margin: 0; font-family: var(--display); font-weight: 700; font-size: 30px; line-height: 1.15; }
  .small { font-size: 12px; }
  .sub { color: var(--text-2); margin-left: 6px; }
  .list { display: flex; flex-direction: column; gap: 8px; }
  .list p { margin: 0; }
  .item { display: flex; gap: 14px; align-items: center; padding: 12px 16px; border-radius: 12px; background: var(--panel); border: 1px solid var(--line); font-size: 14px; }
  .stack { display: flex; flex-direction: column; gap: 2px; flex: 1; min-width: 0; }
  .result { width: 64px; text-align: center; font-size: 11px; padding: 2px 0; border-radius: 6px; border: 1px solid var(--line-2); color: var(--muted); }
  .result.passed { color: var(--ok-text); border-color: var(--ok-border); background: var(--ok-bg); }
  .result.failed { color: var(--warn-text); border-color: var(--warn-border); background: var(--warn-bg); }
  .link { border: 0; background: none; padding: 0; color: var(--accent-text); cursor: pointer; }
  .link:hover { color: var(--link-hover); }
</style>
