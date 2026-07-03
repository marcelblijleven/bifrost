<script lang="ts">
  import type { NotificationConfig } from '$lib/types'

  let { initial }: { initial?: NotificationConfig } = $props()

  let onFailureUrl = $state(initial?.on_failure_url ?? '')
  let onApprovalUrl = $state(initial?.on_approval_url ?? '')
  let headersRaw = $state(
    Object.entries(initial?.headers ?? {})
      .map(([k, v]) => `${k}: ${v}`)
      .join('\n')
  )

  function parseHeaders(text: string): Record<string, string> {
    const out: Record<string, string> = {}
    for (const line of text.split('\n')) {
      const idx = line.indexOf(':')
      if (idx <= 0) continue
      const key = line.slice(0, idx).trim()
      const val = line.slice(idx + 1).trim()
      if (key) out[key] = val
    }
    return out
  }
</script>

<div class="rounded-lg border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 p-5 space-y-4">
  <h2 class="text-sm font-medium text-zinc-700 dark:text-zinc-300">Notifications</h2>
  <div>
    <label for="on_failure_url" class="mb-1.5 block text-xs text-zinc-400 dark:text-zinc-500">On failure webhook URL</label>
    <input
      id="on_failure_url"
      type="url"
      bind:value={onFailureUrl}
      placeholder="https://your-service.example.com/webhook"
      class="w-full rounded-md border border-zinc-300 dark:border-zinc-700 bg-zinc-50 dark:bg-zinc-800 px-3 py-2 text-sm text-zinc-900 dark:text-white placeholder-zinc-400 dark:placeholder-zinc-500 focus:outline-none focus:ring-2 focus:ring-brand-500"
    />
    <p class="mt-1 text-xs text-zinc-400 dark:text-zinc-500">POST request with a JSON payload sent when a pipeline run fails or is cancelled. Works with any service that accepts JSON — use an intermediary for Slack or Discord.</p>
  </div>
  <div>
    <label for="on_approval_url" class="mb-1.5 block text-xs text-zinc-400 dark:text-zinc-500">On approval requested webhook URL</label>
    <input
      id="on_approval_url"
      type="url"
      bind:value={onApprovalUrl}
      placeholder="https://your-service.example.com/webhook"
      class="w-full rounded-md border border-zinc-300 dark:border-zinc-700 bg-zinc-50 dark:bg-zinc-800 px-3 py-2 text-sm text-zinc-900 dark:text-white placeholder-zinc-400 dark:placeholder-zinc-500 focus:outline-none focus:ring-2 focus:ring-brand-500"
    />
    <p class="mt-1 text-xs text-zinc-400 dark:text-zinc-500">POST request sent when a pipeline pauses waiting on a human approval, so someone finds out without watching the dashboard.</p>
  </div>
  <div>
    <label for="notification_headers_raw" class="mb-1.5 block text-xs text-zinc-400 dark:text-zinc-500">Headers</label>
    <textarea
      id="notification_headers_raw"
      rows="2"
      bind:value={headersRaw}
      placeholder={"Authorization: Bearer token\nX-Custom: value"}
      class="w-full rounded-md border border-zinc-300 dark:border-zinc-700 bg-zinc-50 dark:bg-zinc-800 px-3 py-2 font-mono text-sm text-zinc-900 dark:text-white placeholder-zinc-400 dark:placeholder-zinc-600 focus:outline-none focus:ring-2 focus:ring-brand-500"
    ></textarea>
    <p class="mt-1 text-xs text-zinc-400 dark:text-zinc-500">One "Name: value" per line, sent with both notification requests. Useful for authenticated endpoints.</p>
  </div>
  <input type="hidden" name="on_failure_url" value={onFailureUrl} />
  <input type="hidden" name="on_approval_url" value={onApprovalUrl} />
  <input type="hidden" name="notification_headers" value={JSON.stringify(parseHeaders(headersRaw))} />
</div>
