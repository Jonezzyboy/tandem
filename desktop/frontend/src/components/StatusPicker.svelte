<script lang="ts">
  import Icon from './Icon.svelte'

  let { selected, options, onchange }: { selected: string[]; options: string[] | null; onchange: (next: string[]) => void } = $props()

  let query = $state('')
  let open = $state(false)
  let active = $state(0)
  let input = $state<HTMLInputElement>()

  const matches = $derived((options ?? []).filter((s) => !selected.includes(s) && s.toLowerCase().includes(query.trim().toLowerCase())))
  const known = $derived(new Set(options ?? []))

  function add(st: string) {
    onchange([...selected, st])
    query = ''
    active = 0
    input?.focus()
  }

  function remove(st: string) {
    onchange(selected.filter((s) => s !== st))
  }

  function onKey(e: KeyboardEvent) {
    if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
      e.preventDefault()
      open = true
      if (matches.length) active = (active + (e.key === 'ArrowDown' ? 1 : -1) + matches.length) % matches.length
    } else if (e.key === 'Enter' && open && matches[active]) {
      e.preventDefault()
      add(matches[active])
    } else if (e.key === 'Escape') {
      open = false
    } else if (e.key === 'Backspace' && !query && selected.length) {
      remove(selected[selected.length - 1])
    }
  }
</script>

<div class="picker">
  <div class="box" class:focused={open}>
    {#each selected as st (st)}
      {@const missing = options !== null && !known.has(st)}
      <span class="chip" class:missing title={missing ? `${st} isn't a status in your Jira` : undefined}>
        {#if missing}<Icon name="alert" size={12} />{/if}{st}
        <button class="remove" aria-label="Remove {st}" onclick={() => remove(st)}><Icon name="close" size={12} /></button>
      </span>
    {/each}
    <input
      bind:this={input}
      bind:value={query}
      placeholder={options === null ? 'Reading statuses…' : selected.length ? 'Add another…' : 'Add a status…'}
      disabled={options === null}
      role="combobox"
      aria-label="Add a status"
      aria-expanded={open && matches.length > 0}
      aria-controls="status-options"
      onfocus={() => (open = true)}
      oninput={() => { open = true; active = 0 }}
      onblur={() => setTimeout(() => (open = false), 120)}
      onkeydown={onKey}
    />
  </div>
  {#if open && options !== null}
    <div class="menu" id="status-options" role="listbox">
      {#each matches as st, i (st)}
        <button class="option" class:active={i === active} role="option" aria-selected={i === active}
          onmousedown={(e) => { e.preventDefault(); add(st) }} onmouseenter={() => (active = i)}>{st}</button>
      {:else}
        <span class="empty">{query ? `No status matches "${query}"` : 'Every status is chosen'}</span>
      {/each}
    </div>
  {/if}
</div>

<style>
  .picker { position: relative; max-width: 520px; }
  .box {
    display: flex; flex-flow: row wrap; gap: 6px; align-items: center; min-height: 40px; padding: 5px 8px;
    border-radius: 8px; border: 1px solid var(--line-2); background: var(--nav); cursor: text;
  }
  .box.focused { border-color: var(--accent); }
  .chip {
    display: inline-flex; align-items: center; gap: 4px; padding: 2px 4px 2px 9px; border-radius: 6px; font-size: 13px;
    border: 1px solid var(--accent); background: var(--accent-bg); color: var(--accent-text);
  }
  .chip.missing { border-color: var(--warn-border); background: var(--warn-bg); color: var(--warn-text); padding-left: 7px; }
  .remove { display: grid; place-items: center; width: 20px; height: 20px; border: 0; border-radius: 4px; background: transparent; color: inherit; cursor: pointer; }
  .remove:hover { background: color-mix(in srgb, currentColor 18%, transparent); }
  input { flex: 1; min-width: 120px; border: 0; outline: none; background: transparent; color: var(--text); font: inherit; font-size: 13px; padding: 4px 2px; }
  .menu {
    position: absolute; z-index: 20; top: calc(100% + 4px); left: 0; right: 0; max-height: 240px; overflow-y: auto;
    display: flex; flex-direction: column; padding: 4px; border-radius: 10px; border: 1px solid var(--line-2);
    background: var(--raised); box-shadow: 0 12px 28px var(--shadow);
  }
  .option { text-align: left; padding: 7px 10px; border: 0; border-radius: 6px; background: transparent; color: var(--text-2); font: inherit; font-size: 13px; cursor: pointer; }
  .option.active { background: var(--hover); color: var(--text); }
  .empty { padding: 8px 10px; font-size: 13px; color: var(--muted); }
</style>
