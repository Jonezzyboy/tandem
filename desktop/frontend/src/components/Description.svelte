<script lang="ts">
  let { text }: { text: string } = $props()

  // Jira code blocks and tables arrive fenced in ``` lines; their columns only line up unwrapped in a monospace font.
  const parts = $derived(text.split(/^```\n([\s\S]*?)\n```$/m).map((body, i) => ({ code: i % 2 === 1, body: i % 2 ? body : body.trim() })).filter((p) => p.body))
</script>

{#each parts as p, i (i)}
  {#if p.code}<pre class="mono">{p.body}</pre>{:else}<div>{p.body}</div>{/if}
{/each}

<style>
  div { white-space: pre-wrap; }
  div + *, pre + * { margin-top: 0.8em; }
  pre {
    margin: 0; padding: 10px 12px; border-radius: 8px; background: var(--raised); border: 1px solid var(--line);
    font-size: 12px; line-height: 1.45; white-space: pre; overflow-x: auto; font-variant-ligatures: none;
  }
</style>
