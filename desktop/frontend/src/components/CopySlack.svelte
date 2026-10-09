<script lang="ts">
  import { onDestroy } from 'svelte'
  import { ClipboardSetText } from '@wailsjs/runtime/runtime'
  import { fail } from '@lib/state.svelte'
  import Icon from './Icon.svelte'

  let { text, primary = false }: { text: string; primary?: boolean } = $props()

  let copied = $state(false)
  let timer: ReturnType<typeof setTimeout>
  async function copy() {
    try {
      await ClipboardSetText(text)
      copied = true
      clearTimeout(timer)
      timer = setTimeout(() => (copied = false), 2000)
    } catch (e) {
      fail(e)
    }
  }
  onDestroy(() => clearTimeout(timer))
</script>

<button class="btn" class:primary onclick={copy} title={text}>
  <Icon name={copied ? 'check' : 'link'} />{copied ? 'Copied' : 'Copy for Slack'}
</button>
