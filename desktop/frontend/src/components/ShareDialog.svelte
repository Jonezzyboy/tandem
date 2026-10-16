<script lang="ts">
  import { untrack } from 'svelte'
  import { ClipboardSetText } from '@wailsjs/runtime/runtime'
  import { api } from '@lib/api'
  import { fail } from '@lib/state.svelte'
  import type { ChangeView, ShareLinks } from '@lib/types'
  import Icon from './Icon.svelte'

  let { view, onclose }: { view: ChangeView; onclose: () => void } = $props()

  let note = $state('')
  let links = $state<ShareLinks | null>(null)
  let copied = $state('')

  $effect(() => {
    const n = note
    const t = setTimeout(() => api.shareLink(view.id, n).then((l) => (links = l)).catch(fail), untrack(() => links) ? 300 : 0)
    return () => clearTimeout(t)
  })

  const unopened = $derived(view.legs.filter((l) => !l.pr).map((l) => l.name))
  const slack = $derived(links ? `Ready to test: ${view.id}${view.title ? ` · ${view.title}` : ''}\n${links.web}` : '')

  async function copy(what: 'link' | 'slack') {
    if (!links) return
    await ClipboardSetText(what === 'link' ? links.web : slack)
    copied = what
    setTimeout(() => (copied = ''), 1600)
  }

  function onKey(e: KeyboardEvent) {
    if (e.key === 'Escape') onclose()
  }
</script>

<svelte:window onkeydown={onKey} />

<div class="scrim" role="presentation" onclick={onclose}></div>
<div class="sheet" role="dialog" aria-modal="true" aria-labelledby="share-title">
  <div class="top">
    <div>
      <div class="mono muted small">{view.id} / share</div>
      <h2 id="share-title">Share with a tester</h2>
    </div>
    <button class="icon-btn" aria-label="Close" onclick={onclose}><Icon name="close" /></button>
  </div>
  <p class="muted lead">The link opens this change in the tester's Tandem with every repo's branch, the PRs and the Jira ticket filled in, so they don't have to find them.</p>
  <label class="field">
    <span>Note for the tester <span class="muted small">· optional</span></span>
    <textarea class="input" rows="3" bind:value={note} placeholder="Setup, test data, feature flags…"></textarea>
  </label>
  <div class="field">
    <span>What it sets up</span>
    <ul class="repos mono small">
      {#each view.legs as l (l.repo)}
        <li><span>{l.repo}</span><span class="muted">{view.branch}{l.pr ? ` · #${l.pr.number}` : ''}</span></li>
      {/each}
    </ul>
    {#if unopened.length}<span class="small warn">No PR yet for {unopened.join(', ')}: push the branch so the tester can get it.</span>{/if}
  </div>
  <div class="link mono small selectable">{links?.web ?? 'Making the link…'}</div>
  <div class="foot">
    <button class="btn" disabled={!links} onclick={() => copy('slack')}>{copied === 'slack' ? 'Copied' : 'Copy for Slack'}</button>
    <button class="btn primary" disabled={!links} onclick={() => copy('link')}><Icon name="link" />{copied === 'link' ? 'Copied' : 'Copy link'}</button>
  </div>
</div>

<style>
  .scrim { position: fixed; inset: 0; background: var(--scrim); z-index: 30; }
  .sheet {
    position: fixed; top: 12vh; left: 50%; transform: translateX(-50%); z-index: 31; width: min(600px, calc(100vw - 48px));
    max-height: 80vh; overflow: auto; display: flex; flex-direction: column; gap: 16px; padding: 22px 24px;
    background: var(--bg); border: 1px solid var(--line-2); border-radius: 16px; box-shadow: 0 24px 60px var(--shadow);
  }
  .top { display: flex; justify-content: space-between; align-items: flex-start; }
  h2 { margin: 4px 0 0; font-family: var(--display); font-weight: 700; font-size: 24px; }
  .small { font-size: 12px; }
  .lead { margin: 0; font-size: 13.5px; line-height: 1.55; }
  .field { display: flex; flex-direction: column; gap: 6px; font-size: 13px; }
  .repos { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 4px; }
  .repos li { display: flex; justify-content: space-between; gap: 12px; padding: 6px 10px; border: 1px solid var(--line); border-radius: 8px; }
  .link { padding: 10px 12px; border-radius: 8px; background: var(--panel); border: 1px solid var(--line); overflow-wrap: anywhere; color: var(--text-2); max-height: 84px; overflow-y: auto; }
  .foot { display: flex; justify-content: flex-end; gap: 10px; }
</style>
