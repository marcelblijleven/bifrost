<script lang="ts">
  let {
    triggerType = $bindable('push'),
    initialTagPattern = '',
    initialTagPrefix = '',
  }: {
    triggerType?: 'push' | 'tag'
    initialTagPattern?: string
    initialTagPrefix?: string
  } = $props()
</script>

<div class="rounded-lg border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 p-5 space-y-4">
  <h2 class="text-sm font-medium text-zinc-700 dark:text-zinc-300">Trigger</h2>
  <p class="text-xs text-zinc-400 dark:text-zinc-500">
    What starts a pipeline run. An application listens to branch pushes or tag pushes, never both.
  </p>

  <div class="grid gap-4 sm:grid-cols-2">
    <div>
      <label class="block text-xs font-medium text-zinc-500 dark:text-zinc-400 mb-1" for="trigger_type">Trigger type</label>
      <select
        id="trigger_type" name="trigger_type" bind:value={triggerType}
        class="w-full rounded-md border border-zinc-300 dark:border-zinc-700 bg-zinc-50 dark:bg-zinc-800 px-3 py-2 text-sm text-zinc-900 dark:text-white focus:outline-none focus:ring-2 focus:ring-brand-500"
      >
        <option value="push">Push to branch</option>
        <option value="tag">Tag push</option>
      </select>
    </div>

    {#if triggerType === 'tag'}
      <div>
        <label class="block text-xs font-medium text-zinc-500 dark:text-zinc-400 mb-1" for="tag_pattern">Tag pattern</label>
        <input
          id="tag_pattern" name="tag_pattern" type="text" required
          value={initialTagPattern}
          class="w-full rounded-md border border-zinc-300 dark:border-zinc-700 bg-zinc-50 dark:bg-zinc-800 px-3 py-2 text-sm text-zinc-900 dark:text-white placeholder-zinc-400 dark:placeholder-zinc-500 focus:outline-none focus:ring-2 focus:ring-brand-500"
          placeholder="v*"
        />
        <p class="mt-1 text-xs text-zinc-400 dark:text-zinc-500">
          Pushed tags matching this glob start a run. The tagged commit must be reachable from the branch above.
        </p>
      </div>
    {:else}
      <div>
        <label class="block text-xs font-medium text-zinc-500 dark:text-zinc-400 mb-1" for="tag_prefix">Tag prefix</label>
        <input
          id="tag_prefix" name="tag_prefix" type="text"
          value={initialTagPrefix}
          class="w-full rounded-md border border-zinc-300 dark:border-zinc-700 bg-zinc-50 dark:bg-zinc-800 px-3 py-2 text-sm text-zinc-900 dark:text-white placeholder-zinc-400 dark:placeholder-zinc-500 focus:outline-none focus:ring-2 focus:ring-brand-500"
          placeholder="frontend-"
        />
        <p class="mt-1 text-xs text-zinc-400 dark:text-zinc-500">
          Optional. Namespaces this application's release tags (e.g. frontend-v1.2.3) so several applications can release from one repository. Leave empty for single-application repositories.
        </p>
      </div>
    {/if}
  </div>

  {#if triggerType === 'tag'}
    <p class="text-xs text-zinc-400 dark:text-zinc-500">
      The pushed tag provides the version, so the pipeline may not contain semver or tag steps.
      Recreating a tag at a different commit blocks the application, like a force push does.
    </p>
  {/if}
</div>
