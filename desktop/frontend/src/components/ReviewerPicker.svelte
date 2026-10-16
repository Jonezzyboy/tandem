<script lang="ts">
  import { onMount, tick } from 'svelte'
  import { api, errorText } from '@lib/api'
  import type { Reviewer } from '@lib/types'
  import Icon from './Icon.svelte'

  // value is the comma-separated list the PR request takes: "user, org/team".
  let { value = $bindable(), changeId, id }: { value: string; changeId: string; id: string } = $props()

  let suggestions = $state<Reviewer[] | null>(null)
  let loadError = $state('')
  let query = $state('')
  let open = $state(false)
  let active = $state(0)
  let input = $state<HTMLInputElement>()

  const chosen = $derived(value.split(',').map((r) => r.trim()).filter(Boolean))
  const matches = $derived.by(() => {
    const q = query.trim().toLowerCase()
    const taken = new Set(chosen.map((c) => c.toLowerCase()))
    return (suggestions ?? [])
      .filter((r) => !taken.has(r.login.toLowerCase()))
      .filter((r) => !q || r.login.toLowerCase().includes(q) || r.name.toLowerCase().includes(q))
      .slice(0, 8)
  })

  onMount(() => {
    api.reviewers(changeId).then((r) => (suggestions = r)).catch((e) => { suggestions = []; loadError = errorText(e) })
  })

  $effect(() => {
    query
    active = 0
  })

  function add(login: string) {
    login = login.trim().replace(/^@/, '')
    if (!login || chosen.some((c) => c.toLowerCase() === login.toLowerCase())) return
    value = [...chosen, login].join(', ')
    query = ''
  }

  function remove(login: string) {
    value = chosen.filter((c) => c !== login).join(', ')
    input?.focus()
  }

  async function onKey(e: KeyboardEvent) {
    if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
      e.preventDefault()
      open = true
      if (matches.length) active = (active + (e.key === 'ArrowDown' ? 1 : matches.length - 1)) % matches.length
    } else if (e.key === 'Enter' || e.key === 'Tab' || e.key === ',') {
      const pick = open && matches[active] ? matches[active].login : query
      if (!pick.trim()) return
      e.preventDefault()
      add(pick)
      await tick()
      open = true
    } else if (e.key === 'Backspace' && !query && chosen.length) {
      remove(chosen[chosen.length - 1])
    } else if (e.key === 'Escape' && open) {
      e.stopPropagation()
      open = false
    }
  }
</script>

<div class="picker" class:focused={open}>
  <div class="box" role="presentation" onclick={() => input?.focus()}>
    {#each chosen as c (c)}
      <span class="chip">
        <span class="mono">{c}</span>
        <button type="button" aria-label="Remove {c}" onclick={(e) => { e.stopPropagation(); remove(c) }}><Icon name="close" size={12} /></button>
      </span>
    {/each}
    <input {id} bind:this={input} bind:value={query} onkeydown={onKey} onfocus={() => (open = true)} onblur={() => setTimeout(() => (open = false), 120)}
      placeholder={chosen.length ? '' : 'Add people or teams'} autocomplete="off" spellcheck="false"
      role="combobox" aria-expanded={open && matches.length > 0} aria-controls="{id}-list" />
  </div>
  {#if open && (matches.length || suggestions === null || loadError)}
    <div class="menu" id="{id}-list" role="listbox">
      {#if suggestions === null}
        <div class="note muted"><Icon name="running" size={13} spin />Finding reviewers…</div>
      {:else if loadError}
        <div class="note warn">Couldn't load suggestions; type a login. {loadError}</div>
      {/if}
      {#each matches as r, i (r.login)}
        <button type="button" class="option" role="option" aria-selected={i === active}
          onmousedown={(e) => { e.preventDefault(); add(r.login) }} onmousemove={() => (active = i)}>
          <span class="mono login">{r.login}</span>
          {#if r.name}<span class="muted name">{r.name}</span>{/if}
          {#if r.recent}<span class="tag" title="Reviewed {r.recent} of your recent PRs in these repos">recent</span>{:else if r.team}<span class="tag">team</span>{/if}
        </button>
      {/each}
    </div>
  {/if}
</div>

<style>
  .picker { position: relative; }
  .box {
    display: flex; flex-wrap: wrap; align-items: center; gap: 6px; min-height: 40px; padding: 5px 8px;
    background: var(--bg); border: 1px solid var(--line-2); border-radius: 8px; cursor: text;
  }
  .picker.focused .box { outline: 2px solid var(--accent); outline-offset: -1px; border-color: transparent; }
  .box input { flex: 1; min-width: 120px; border: 0; background: none; outline: none; padding: 4px; font-size: 13.5px; color: var(--text); }
  .chip { display: inline-flex; align-items: center; gap: 5px; padding: 2px 4px 2px 8px; border-radius: 6px; background: var(--raised); border: 1px solid var(--line); font-size: 12.5px; }
  .chip button { display: grid; place-items: center; width: 18px; height: 18px; border: 0; border-radius: 4px; background: none; color: var(--muted); cursor: pointer; }
  .chip button:hover { background: var(--hover); color: var(--text); }
  .menu {
    position: absolute; left: 0; right: 0; top: calc(100% + 4px); z-index: 5; padding: 4px; max-height: 260px; overflow-y: auto;
    background: var(--bg); border: 1px solid var(--line-2); border-radius: 10px; box-shadow: 0 12px 30px var(--shadow);
  }
  .option { display: flex; align-items: baseline; gap: 10px; width: 100%; padding: 7px 10px; border: 0; border-radius: 6px; background: none; color: var(--text); text-align: left; font-size: 13px; cursor: pointer; }
  .option[aria-selected='true'] { background: var(--selected); }
  .login { flex-shrink: 0; }
  .name { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 12.5px; }
  .tag { margin-left: auto; font-size: 11px; color: var(--accent-text); }
  .note { display: flex; align-items: center; gap: 8px; padding: 8px 10px; font-size: 12.5px; }
</style>
