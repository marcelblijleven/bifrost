<script lang="ts">
  import { copyToClipboard } from '$lib/clipboard'

  // 'create' shows a required input; 'edit' keeps the stored secret untouched
  // until a new one is generated (the backend keeps the old secret when the
  // submitted value is empty).
  let { mode = 'create' }: { mode?: 'create' | 'edit' } = $props()

  let webhookSecret = $state('')
  let copied = $state(false)

  function generateSecret() {
    const bytes = crypto.getRandomValues(new Uint8Array(20))
    webhookSecret = btoa(String.fromCharCode(...bytes))
      .replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '')
  }

  async function copySecret() {
    if (!webhookSecret) return
    await copyToClipboard(webhookSecret)
    copied = true
    setTimeout(() => (copied = false), 2000)
  }
</script>

<div>
  <label class="block text-xs font-medium text-zinc-500 dark:text-zinc-400 mb-1" for="webhook_secret">
    Webhook secret
  </label>
  <div class="flex gap-2">
    {#if mode === 'create' || webhookSecret}
      <input
        id="webhook_secret" name="webhook_secret" type="text" required={mode === 'create'}
        bind:value={webhookSecret}
        class="flex-1 rounded-md border border-zinc-300 dark:border-zinc-700 bg-zinc-50 dark:bg-zinc-800 px-3 py-2 text-sm text-zinc-900 dark:text-white placeholder-zinc-400 dark:placeholder-zinc-500 focus:outline-none focus:ring-2 focus:ring-brand-500 font-mono"
        placeholder="Generate or enter a secret"
      />
    {:else}
      <input type="hidden" name="webhook_secret" value="" />
      <div class="flex-1 rounded-md border border-zinc-300 dark:border-zinc-700 bg-zinc-100/50 dark:bg-zinc-800/50 px-3 py-2 text-sm text-zinc-400 dark:text-zinc-500 italic select-none">
        Existing secret kept. Generate a new one to rotate it.
      </div>
    {/if}
    <button
      type="button"
      onclick={generateSecret}
      class="shrink-0 rounded-md border border-zinc-300 dark:border-zinc-700 bg-zinc-100 dark:bg-zinc-800 px-3 py-2 text-xs font-medium text-zinc-700 dark:text-zinc-300 hover:border-zinc-400 dark:hover:border-zinc-500 hover:text-zinc-900 dark:hover:text-white transition-colors"
    >
      Generate
    </button>
    <button
      type="button"
      onclick={copySecret}
      disabled={!webhookSecret}
      class="shrink-0 rounded-md border border-zinc-300 dark:border-zinc-700 bg-zinc-100 dark:bg-zinc-800 px-3 py-2 text-xs font-medium transition-colors disabled:opacity-30 {copied ? 'text-green-400 border-green-700' : 'text-zinc-700 dark:text-zinc-300 hover:border-zinc-400 dark:hover:border-zinc-500 hover:text-zinc-900 dark:hover:text-white'}"
    >
      {copied ? 'Copied!' : 'Copy'}
    </button>
  </div>
</div>
