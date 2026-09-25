<script lang="ts">
  import { onMount, untrack } from 'svelte'
  import { api } from '@lib/api'
  import { fail, log } from '@lib/state.svelte'
  import { prefs } from '@lib/settings.svelte'
  import type { ChangeView, PRPreview, PRRequest } from '@lib/types'
  import Icon from './Icon.svelte'

  let { view, onclose }: { view: ChangeView; onclose: () => void } = $props()

  // The form is seeded once; background refreshes of view must not overwrite typing.
  let title = $state(untrack(() => view.title))
  let body = $state(untrack(() => view.body))
  let reviewers = $state(untrack(() => (view.reviewers ?? []).join(', ')))
  let draft = $state(untrack(() => prefs.settings.draftPRs))
  const hasDrafts = $derived(view.legs.some((l) => l.pr?.draft && l.pr.state === 'OPEN'))
  let ready = $state(untrack(() => hasDrafts))
  let forceWithLease = $state(false)
  let preview = $state<PRPreview | null>(null)
  let planning = $state(false)
  let publishing = $state(false)
  let done = $state<PRPreview | null>(null)

  const acting = $derived((preview?.items ?? []).filter((i) => i.action !== 'skip' && !i.error))
  const creating = $derived(acting.filter((i) => i.action === 'create').length)

  function request(): PRRequest {
    return {
      title,
      body,
      reviewers: reviewers.split(',').map((r) => r.trim()).filter(Boolean),
      draft,
      ready,
      forceWithLease,
    }
  }

  async function plan() {
    planning = true
    try {
      preview = await api.planPRs(view.id, request())
    } catch (e) {
      fail(e)
    } finally {
      planning = false
    }
  }

  async function publish() {
    publishing = true
    try {
      done = await api.publishPRs(view.id, request())
      for (const it of done.items) {
        if (it.error) log(view.id, `${it.name}: ${it.error}`, 'warn')
        else if (it.action !== 'skip' && it.url) log(view.id, `${it.name}: ${it.action === 'create' ? 'opened' : it.ready ? 'ready for review' : 'updated'} ${it.url}`, 'ok')
      }
    } catch (e) {
      fail(e)
    } finally {
      publishing = false
    }
  }

  onMount(plan)

  function onKey(e: KeyboardEvent) {
    if (e.key === 'Escape') onclose()
  }

  const relatedPreview = $derived.by(() => {
    const items = preview?.items ?? []
    const levels = [...new Set(items.map((i) => i.level))].sort((a, b) => a - b)
    return levels.map((lvl, i) =>
      `${i + 1}. ${items.filter((it) => it.level === lvl).map((it) => it.url ? it.url.replace('https://github.com/', '').replace('/pull/', '#') : `${it.repo}#…`).join(' · ')}`,
    )
  })
</script>

<svelte:window onkeydown={onKey} />

<div class="scrim" role="presentation" onclick={onclose}></div>
<div class="sheet" role="dialog" aria-modal="true" aria-labelledby="composer-title">
  <div class="top">
    <div>
      <div class="mono muted small">{view.id} / pull requests</div>
      <h2 id="composer-title">{done ? 'Published' : 'Write once, open every PR'}</h2>
    </div>
    <button class="icon-btn" aria-label="Close" onclick={onclose}><Icon name="close" /></button>
  </div>

  {#if done}
    <div class="results">
      {#each done.items as it (it.repo)}
        <div class="result">
          {#if it.error}<Icon name="x" color="var(--warn)" />{:else if it.url}<Icon name="check" color="var(--ok)" />{:else}<Icon name="clock" color="var(--muted)" />{/if}
          <span class="mono name">{it.repo}</span>
          {#if it.error}
            <span class="warn small selectable">{it.error}</span>
          {:else if it.url}
            <a href={it.url} class="mono small" onclick={(e) => { e.preventDefault(); api.openURL(it.url) }}>{it.url}</a>
          {:else}
            <span class="muted small">{it.note}</span>
          {/if}
        </div>
      {/each}
      <div class="foot"><button class="btn" onclick={onclose}>Done</button></div>
    </div>
  {:else}
    <div class="body">
      <div class="form">
        <div class="eyebrow">Shared across every PR</div>
        <div class="field">
          <label for="pr-title">Title</label>
          <input id="pr-title" class="input" bind:value={title} placeholder="What this change does" />
          <span class="muted small">Sent as “{view.id} {title.replace(view.id, '').trim() || '…'}”</span>
        </div>
        <div class="field grow">
          <label for="pr-body">Description</label>
          <textarea id="pr-body" class="input mono grow" bind:value={body} placeholder="What was wrong, what changed, how it was verified"></textarea>
        </div>
        <div class="field">
          <label for="pr-reviewers">Reviewers</label>
          <input id="pr-reviewers" class="input" bind:value={reviewers} placeholder="user, org/team" />
        </div>
        <div class="toggles">
          <label class="toggle"><input type="checkbox" bind:checked={draft} onchange={plan} />Open new PRs as drafts</label>
          {#if hasDrafts}
            <label class="toggle"><input type="checkbox" bind:checked={ready} onchange={plan} />Mark drafts ready for review</label>
          {/if}
          <label class="toggle" title="Needed after Sync rebased a branch that was already pushed">
            <input type="checkbox" bind:checked={forceWithLease} />Push with --force-with-lease
          </label>
        </div>
      </div>

      <div class="plan">
        <div class="plan-head">
          <div class="eyebrow">Per repo, in merge order</div>
          <button class="icon-btn" aria-label="Re-check plan" title="Re-check" onclick={plan}>
            <Icon name="refresh" spin={planning} />
          </button>
        </div>
        {#if !preview}
          <p class="muted small">Reading each leg…</p>
        {:else}
          {#each preview.items as it (it.repo)}
            <div class="item" class:skip={it.action === 'skip'}>
              <div class="level mono">{it.level}</div>
              <div class="stack">
                <div class="mono">{it.repo}</div>
                {#if it.error}
                  <div class="small warn selectable">{it.error}</div>
                {:else}
                  <div class="small" class:muted={it.action === 'skip'}>{it.note}</div>
                  {#if it.dirty && it.action !== 'skip'}<div class="small warn">{it.dirty} uncommitted files will be left out</div>{/if}
                {/if}
              </div>
            </div>
          {/each}
          <div class="related mono">
            <div class="muted">Kept in every PR · numbers fill in as they open</div>
            <div>### Related PRs — merge in this order</div>
            {#each relatedPreview as line}<div>{line}</div>{/each}
          </div>
        {/if}
      </div>
    </div>
    <div class="foot">
      <span class="muted small">Pushes each leg with commits, opens missing PRs, then refreshes the Related PRs block.</span>
      <button class="btn primary" disabled={publishing || planning || acting.length === 0} onclick={publish}>
        <Icon name="send" spin={publishing} />
        {#if acting.length === 0}Nothing to publish{:else if creating === acting.length}Open {creating} PR{creating === 1 ? '' : 's'}{:else if creating === 0}Update {acting.length} PR{acting.length === 1 ? '' : 's'}{:else}Publish {acting.length} PR{acting.length === 1 ? '' : 's'}{/if}
      </button>
    </div>
  {/if}
</div>

<style>
  .scrim { position: fixed; inset: 0; background: var(--scrim); }
  .sheet {
    position: fixed; top: 48px; bottom: 32px; left: 50%; transform: translateX(-50%);
    width: min(1120px, calc(100vw - 80px)); background: var(--bg); border: 1px solid var(--line-2);
    border-radius: 16px; display: flex; flex-direction: column; box-shadow: 0 24px 60px var(--shadow);
  }
  .top { display: flex; justify-content: space-between; align-items: flex-start; padding: 22px 24px 0; }
  h2 { margin: 4px 0 0; font-family: var(--display); font-weight: 700; font-size: 26px; }
  .small { font-size: 12px; }
  .body { flex: 1; min-height: 0; display: grid; grid-template-columns: 460px minmax(0, 1fr); gap: 20px; padding: 20px 24px; }
  .form, .plan { display: flex; flex-direction: column; gap: 14px; min-height: 0; }
  .form { padding: 18px; background: var(--panel); border: 1px solid var(--line); border-radius: 12px; }
  .grow { flex: 1; min-height: 0; }
  .toggles { display: flex; flex-direction: column; gap: 8px; }
  .toggle { display: flex; gap: 10px; align-items: center; font-size: 13px; cursor: pointer; }
  .toggle input { width: 16px; height: 16px; accent-color: var(--accent); }
  .plan { overflow: auto; }
  .plan-head { display: flex; justify-content: space-between; align-items: center; }
  .item { display: flex; gap: 14px; padding: 13px 16px; background: var(--panel); border: 1px solid var(--line); border-radius: 10px; }
  .item.skip { background: transparent; }
  .level { width: 22px; height: 22px; border-radius: 6px; background: var(--text); color: var(--bg); display: grid; place-items: center; font-size: 12px; flex-shrink: 0; }
  .stack { display: flex; flex-direction: column; gap: 4px; min-width: 0; }
  .related {
    display: flex; flex-direction: column; gap: 4px; padding: 14px; border: 1px dashed var(--line-2);
    border-radius: 10px; background: var(--nav); font-size: 12.5px; color: var(--text-2);
  }
  .foot { display: flex; justify-content: space-between; align-items: center; gap: 16px; padding: 14px 24px 20px; border-top: 1px solid var(--line); }
  .results { display: flex; flex-direction: column; gap: 10px; padding: 20px 24px 0; flex: 1; }
  .results .foot { margin-top: auto; padding-left: 0; padding-right: 0; justify-content: flex-end; }
  .result { display: flex; gap: 12px; align-items: center; padding: 12px 14px; background: var(--panel); border: 1px solid var(--line); border-radius: 10px; }
  .name { width: 280px; flex-shrink: 0; }
</style>
