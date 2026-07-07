<script lang="ts">
  import type { SkipConditions } from '$lib/types'

  let { initial }: { initial?: SkipConditions } = $props()

  let commitPatterns = $state((initial?.commit_patterns ?? []).join('\n'))
  let pathsIgnore = $state((initial?.paths_ignore ?? []).join('\n'))
  let pathsInclude = $state((initial?.paths_include ?? []).join('\n'))
  let skipBackfill = $state(initial?.skip_backfill ?? false)

  function parseLines(s: string): string[] {
    return s.split('\n').map(l => l.trim()).filter(Boolean)
  }
</script>

<div class="rounded-lg border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 p-5 space-y-4">
  <h2 class="text-sm font-medium text-zinc-700 dark:text-zinc-300">Skip conditions</h2>
  <p class="text-xs text-zinc-400 dark:text-zinc-500">
    Webhook pushes that match any active condition are recorded as <span class="font-mono text-zinc-500 dark:text-zinc-400">skipped</span> without running the pipeline.
  </p>
  <div>
    <label for="skip_commit_patterns" class="mb-1.5 block text-xs text-zinc-500 dark:text-zinc-400">
      Commit message patterns
    </label>
    <textarea
      id="skip_commit_patterns"
      rows="3"
      bind:value={commitPatterns}
      placeholder={"[skip ci]\n[docs only]"}
      class="w-full rounded-md border border-zinc-300 dark:border-zinc-700 bg-zinc-50 dark:bg-zinc-800 px-3 py-2 text-sm text-zinc-900 dark:text-white placeholder-zinc-400 dark:placeholder-zinc-600 focus:outline-none focus:ring-2 focus:ring-brand-500 font-mono"
    ></textarea>
    <p class="mt-1 text-xs text-zinc-400 dark:text-zinc-500">One string per line. Skip if the commit message contains any of these.</p>
  </div>
  <div>
    <label for="skip_paths_ignore" class="mb-1.5 block text-xs text-zinc-500 dark:text-zinc-400">
      Paths to ignore
    </label>
    <textarea
      id="skip_paths_ignore"
      rows="3"
      bind:value={pathsIgnore}
      placeholder={"docs/**\n*.md\n*.txt"}
      class="w-full rounded-md border border-zinc-300 dark:border-zinc-700 bg-zinc-50 dark:bg-zinc-800 px-3 py-2 text-sm text-zinc-900 dark:text-white placeholder-zinc-400 dark:placeholder-zinc-600 focus:outline-none focus:ring-2 focus:ring-brand-500 font-mono"
    ></textarea>
    <p class="mt-1 text-xs text-zinc-400 dark:text-zinc-500">One glob per line. Skip if <em>all</em> changed files match. Supports <code class="text-zinc-500 dark:text-zinc-400">*</code>, <code class="text-zinc-500 dark:text-zinc-400">**</code>, <code class="text-zinc-500 dark:text-zinc-400">?</code>.</p>
  </div>
  <div>
    <label for="skip_paths_include" class="mb-1.5 block text-xs text-zinc-500 dark:text-zinc-400">
      Required paths (run only if matched)
    </label>
    <textarea
      id="skip_paths_include"
      rows="3"
      bind:value={pathsInclude}
      placeholder={"src/**\ninternal/**"}
      class="w-full rounded-md border border-zinc-300 dark:border-zinc-700 bg-zinc-50 dark:bg-zinc-800 px-3 py-2 text-sm text-zinc-900 dark:text-white placeholder-zinc-400 dark:placeholder-zinc-600 focus:outline-none focus:ring-2 focus:ring-brand-500 font-mono"
    ></textarea>
    <p class="mt-1 text-xs text-zinc-400 dark:text-zinc-500">One glob per line. Skip if <em>no</em> changed files match. Leave empty to always run.</p>
  </div>
  <div class="border-t border-zinc-200 dark:border-zinc-800 pt-4">
    <label class="flex items-start gap-2.5 cursor-pointer">
      <input
        type="checkbox"
        bind:checked={skipBackfill}
        class="mt-0.5 h-4 w-4 rounded border-zinc-300 dark:border-zinc-700 text-brand-600 focus:ring-brand-500"
      />
      <span>
        <span class="block text-xs text-zinc-600 dark:text-zinc-300">Skip missed-commit backfill</span>
        <span class="mt-0.5 block text-xs text-zinc-400 dark:text-zinc-500">
          When a webhook is missed, don't create a run for each skipped commit. Bifrost syncs straight to the latest pushed commit instead.
        </span>
      </span>
    </label>
  </div>
  <input type="hidden" name="skip_commit_patterns" value={JSON.stringify(parseLines(commitPatterns))} />
  <input type="hidden" name="skip_paths_ignore" value={JSON.stringify(parseLines(pathsIgnore))} />
  <input type="hidden" name="skip_paths_include" value={JSON.stringify(parseLines(pathsInclude))} />
  <input type="hidden" name="skip_backfill" value={String(skipBackfill)} />
</div>
