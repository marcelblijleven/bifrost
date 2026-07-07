<script module lang="ts">
  export type Step = { id: string; type: string; config: Record<string, unknown> }

  // The id only needs to be unique within this page session (each-block key +
  // headersText map key; stripped from the emitted JSON). crypto.randomUUID is
  // not used because it only exists in secure contexts — over plain-HTTP LAN
  // access it is undefined and every caller would throw.
  let nextStepId = 0
  export function newStepId(): string {
    return `step-${Date.now()}-${nextStepId++}`
  }

  // Client-side mirror of the per-step required fields, so a missing value is
  // caught before the round-trip to the server rejects it.
  export function validateSteps(steps: Step[]): string | null {
    for (let i = 0; i < steps.length; i++) {
      const s = steps[i]
      if (s.type === 'dispatch_workflow' && !String(s.config.workflow ?? '').trim()) {
        return `Step ${i + 1} (dispatch_workflow): "workflow" is required`
      }
      if (s.type === 'notify' && !String(s.config.url ?? '').trim()) {
        return `Step ${i + 1} (notify): "url" is required`
      }
    }
    return null
  }
</script>

<script lang="ts">
  type FieldDef = {
    key: string
    label: string
    type: 'text' | 'boolean' | 'number' | 'headers'
    required?: boolean
    placeholder?: string
    hint?: string
  }

  const SCHEMA: Record<string, FieldDef[]> = {
    semver: [
      { key: 'v_prefix', label: 'v prefix', type: 'boolean', hint: 'Prepend "v" to the first tag (e.g. v0.1.0). Existing tags mirror their own prefix automatically.' },
    ],
    tag: [],
    changelog: [],
    approval: [
      { key: 'message', label: 'Message', type: 'text', placeholder: 'Approve release?' },
      { key: 'timeout_hours', label: 'Timeout (hours)', type: 'number', placeholder: '24' },
    ],
    dispatch_workflow: [
      { key: 'workflow', label: 'Workflow file', type: 'text', required: true, placeholder: 'deploy.yml' },
      { key: 'wait', label: 'Wait for completion', type: 'boolean' },
      { key: 'timeout_minutes', label: 'Wait timeout (minutes)', type: 'number', placeholder: '30', hint: 'Only applies when "Wait for completion" is enabled.' },
      { key: 'require_approval', label: 'Require approval before dispatching', type: 'boolean' },
      { key: 'approval_message', label: 'Approval message', type: 'text', placeholder: 'Approve dispatch?' },
      { key: 'approval_timeout_hours', label: 'Approval timeout (hours)', type: 'number', placeholder: '24' },
    ],
    create_release: [
      { key: 'draft', label: 'Create as draft', type: 'boolean' },
      { key: 'prerelease', label: 'Mark as pre-release', type: 'boolean' },
    ],
    notify: [
      { key: 'url', label: 'Webhook URL', type: 'text', required: true, placeholder: 'https://your-service.example.com/webhook' },
      { key: 'headers', label: 'Headers', type: 'headers', placeholder: 'Authorization: Bearer token\nX-Custom: value', hint: 'One "Name: value" per line, sent with the webhook request.' },
    ],
  }

  const LABELS: Record<string, string> = {
    semver:           'Determine semver',
    tag:              'Tag',
    changelog:        'Changelog',
    approval:         'Approval gate',
    dispatch_workflow:'Dispatch workflow',
    create_release:   'Create release',
    notify:           'Notify',
  }

  // Mirrors Requires() in internal/pipeline (registry.go / steps/{tag,changelog,release}.go).
  const REQUIRES: Record<string, string[]> = {
    tag: ['semver'],
    changelog: ['semver'],
    create_release: ['semver'],
  }

  const STEP_TYPES = Object.keys(SCHEMA)

  let {
    steps = $bindable([]),
    excludeTypes = [],
    satisfiedRequires = [],
  }: {
    steps: Step[]
    /** Step types hidden from the add menu. */
    excludeTypes?: string[]
    /** Requirements met outside the pipeline (mirrors Registry.Build). */
    satisfiedRequires?: string[]
  } = $props()

  const availableTypes = $derived(STEP_TYPES.filter((t) => !excludeTypes.includes(t)))

  // Raw textarea contents for headers fields, keyed by "stepId:fieldKey".
  // Typing "Auth" must not be reformatted away mid-keystroke, so the raw text
  // is kept here and only the parsed result is written into step config.
  let headersText = $state<Record<string, string>>({})

  function addStep(type: string) {
    const defaults: Record<string, unknown> = {}
    for (const f of SCHEMA[type] ?? []) {
      if (f.type === 'boolean') {
        defaults[f.key] = f.key === 'v_prefix' ? true : false
      }
    }
    steps = [...steps, { id: newStepId(), type, config: defaults }]
  }

  function removeStep(i: number) {
    steps = steps.filter((_, idx) => idx !== i)
  }

  function moveUp(i: number) {
    if (i === 0 || blockedUp(i)) return
    const s = [...steps]
    ;[s[i - 1], s[i]] = [s[i], s[i - 1]]
    steps = s
  }

  function moveDown(i: number) {
    if (i === steps.length - 1 || blockedDown(i)) return
    const s = [...steps]
    ;[s[i], s[i + 1]] = [s[i + 1], s[i]]
    steps = s
  }

  // Only the adjacent pair needs checking: moves swap one position at a time.
  function blockedUp(i: number): boolean {
    return i > 0 && (REQUIRES[steps[i].type] ?? []).includes(steps[i - 1].type)
  }

  function blockedDown(i: number): boolean {
    return i < steps.length - 1 && (REQUIRES[steps[i + 1].type] ?? []).includes(steps[i].type)
  }

  // Mirrors Registry.Build's ordering check server-side.
  function orderingWarning(i: number): string | null {
    const seen = new Set([...satisfiedRequires, ...steps.slice(0, i).map(s => s.type)])
    for (const need of REQUIRES[steps[i].type] ?? []) {
      if (!seen.has(need)) {
        return `Requires "${LABELS[need] ?? need}" earlier in the pipeline`
      }
    }
    return null
  }

  function getStr(step: Step, key: string): string {
    const v = step.config[key]
    return v == null ? '' : String(v)
  }

  function getBool(step: Step, key: string, defaultVal = false): boolean {
    const v = step.config[key]
    return v == null ? defaultVal : Boolean(v)
  }

  function setField(i: number, key: string, value: unknown) {
    steps = steps.map((s, idx) =>
      idx === i ? { ...s, config: { ...s.config, [key]: value } } : s
    )
  }

  function formatHeaders(v: unknown): string {
    if (v == null || typeof v !== 'object') return ''
    return Object.entries(v as Record<string, unknown>)
      .map(([k, val]) => `${k}: ${String(val)}`)
      .join('\n')
  }

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

  function getHeadersText(step: Step, key: string): string {
    return headersText[`${step.id}:${key}`] ?? formatHeaders(step.config[key])
  }

  function setHeadersField(i: number, step: Step, key: string, text: string) {
    headersText[`${step.id}:${key}`] = text
    const parsed = parseHeaders(text)
    setField(i, key, Object.keys(parsed).length ? parsed : undefined)
  }

  const stepsJson = $derived(JSON.stringify(
    steps.map(s => {
      const cfg = Object.fromEntries(
        Object.entries(s.config).filter(([, v]) => v !== '' && v !== null && v !== undefined)
      )
      return { type: s.type, ...(Object.keys(cfg).length ? { config: cfg } : {}) }
    })
  ))
</script>

{#if steps.length}
  <div class="space-y-2">
    {#each steps as step, i (step.id)}
      {@const schema = SCHEMA[step.type] ?? []}
      {@const warning = orderingWarning(i)}
      <div class="rounded-md border {warning ? 'border-amber-500/60' : 'border-zinc-300 dark:border-zinc-700'} bg-zinc-100 dark:bg-zinc-800">
        <!-- Step header row -->
        <div class="flex items-center gap-2 px-3 py-2">
          <span class="w-5 shrink-0 text-right text-xs text-zinc-400 dark:text-zinc-500">{i + 1}</span>
          <span class="flex-1 text-sm font-medium text-zinc-800 dark:text-zinc-200">{LABELS[step.type] ?? step.type}</span>
          <div class="flex items-center gap-1">
            <button type="button" onclick={() => moveUp(i)}
              class="rounded p-1 text-zinc-400 dark:text-zinc-500 hover:text-zinc-700 dark:hover:text-zinc-300 disabled:opacity-30"
              disabled={i === 0 || blockedUp(i)}
              title={blockedUp(i) ? `Would move this ahead of "${LABELS[steps[i - 1].type] ?? steps[i - 1].type}", which it requires` : undefined}
              aria-label="Move up">
              <svg class="h-3.5 w-3.5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" d="M4.5 15.75l7.5-7.5 7.5 7.5" />
              </svg>
            </button>
            <button type="button" onclick={() => moveDown(i)}
              class="rounded p-1 text-zinc-400 dark:text-zinc-500 hover:text-zinc-700 dark:hover:text-zinc-300 disabled:opacity-30"
              disabled={i === steps.length - 1 || blockedDown(i)}
              title={blockedDown(i) ? `"${LABELS[steps[i + 1].type] ?? steps[i + 1].type}" requires this step earlier in the pipeline` : undefined}
              aria-label="Move down">
              <svg class="h-3.5 w-3.5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" d="M19.5 8.25l-7.5 7.5-7.5-7.5" />
              </svg>
            </button>
            <button type="button" onclick={() => removeStep(i)}
              class="rounded p-1 text-zinc-400 dark:text-zinc-500 hover:text-red-400" aria-label="Remove">
              <svg class="h-3.5 w-3.5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
              </svg>
            </button>
          </div>
        </div>

        {#if warning}
          <p class="px-3 pb-2 -mt-1 text-xs text-amber-500">{warning}</p>
        {/if}

        <!-- Config fields -->
        {#if schema.length}
          <div class="border-t border-zinc-300/60 dark:border-zinc-700/60 px-3 pb-3 pt-2.5 space-y-2.5">
            {#each schema as field (field.key)}
              <div>
                {#if field.type === 'boolean'}
                  <label class="flex items-center gap-2 cursor-pointer select-none">
                    <input
                      type="checkbox"
                      checked={getBool(step, field.key, field.key === 'v_prefix')}
                      onchange={e => setField(i, field.key, (e.target as HTMLInputElement).checked)}
                      class="h-3.5 w-3.5 rounded border-zinc-400 dark:border-zinc-600 bg-zinc-200 dark:bg-zinc-700 accent-brand-500"
                    />
                    <span class="text-xs text-zinc-700 dark:text-zinc-300">{field.label}</span>
                    {#if field.required}<span class="text-red-400 ml-0.5">*</span>{/if}
                  </label>
                {:else if field.type === 'number'}
                  {@const fid = `step-${i}-${field.key}`}
                  <label for={fid} class="block text-xs text-zinc-500 dark:text-zinc-400 mb-1">
                    {field.label}{#if field.required}<span class="text-red-400 ml-0.5">*</span>{/if}
                  </label>
                  <input
                    id={fid}
                    type="number"
                    value={getStr(step, field.key)}
                    placeholder={field.placeholder ?? ''}
                    oninput={e => {
                      const n = parseInt((e.target as HTMLInputElement).value, 10)
                      setField(i, field.key, isNaN(n) ? '' : n)
                    }}
                    class="w-full max-w-[8rem] rounded border border-zinc-300 dark:border-zinc-600 bg-zinc-50 dark:bg-zinc-700 px-2 py-1 text-xs text-zinc-900 dark:text-white placeholder-zinc-400 dark:placeholder-zinc-500 focus:outline-none focus:ring-1 focus:ring-brand-500"
                  />
                {:else if field.type === 'headers'}
                  {@const fid = `step-${i}-${field.key}`}
                  <label for={fid} class="block text-xs text-zinc-500 dark:text-zinc-400 mb-1">
                    {field.label}{#if field.required}<span class="text-red-400 ml-0.5">*</span>{/if}
                  </label>
                  <textarea
                    id={fid}
                    rows="2"
                    value={getHeadersText(step, field.key)}
                    placeholder={field.placeholder ?? ''}
                    oninput={e => setHeadersField(i, step, field.key, (e.target as HTMLTextAreaElement).value)}
                    class="w-full rounded border border-zinc-300 dark:border-zinc-600 bg-zinc-50 dark:bg-zinc-700 px-2 py-1 font-mono text-xs text-zinc-900 dark:text-white placeholder-zinc-400 dark:placeholder-zinc-500 focus:outline-none focus:ring-1 focus:ring-brand-500"
                  ></textarea>
                {:else}
                  {@const fid = `step-${i}-${field.key}`}
                  <label for={fid} class="block text-xs text-zinc-500 dark:text-zinc-400 mb-1">
                    {field.label}{#if field.required}<span class="text-red-400 ml-0.5">*</span>{/if}
                  </label>
                  <input
                    id={fid}
                    type="text"
                    value={getStr(step, field.key)}
                    placeholder={field.placeholder ?? ''}
                    oninput={e => setField(i, field.key, (e.target as HTMLInputElement).value)}
                    class="w-full rounded border border-zinc-300 dark:border-zinc-600 bg-zinc-50 dark:bg-zinc-700 px-2 py-1 text-xs text-zinc-900 dark:text-white placeholder-zinc-400 dark:placeholder-zinc-500 focus:outline-none focus:ring-1 focus:ring-brand-500"
                  />
                {/if}
                {#if field.hint}
                  <p class="mt-0.5 text-xs text-zinc-400 dark:text-zinc-500">{field.hint}</p>
                {/if}
              </div>
            {/each}
          </div>
        {/if}
      </div>
    {/each}
  </div>
{:else}
  <div class="rounded-lg border border-dashed border-zinc-300 dark:border-zinc-700 p-6 text-center">
    <p class="text-sm text-zinc-400 dark:text-zinc-500">No steps added yet.</p>
  </div>
{/if}

<div class="flex flex-wrap gap-2 {steps.length ? 'mt-2' : 'mt-3'}">
  {#each availableTypes as type (type)}
    <button
      type="button"
      onclick={() => addStep(type)}
      class="rounded-md border border-zinc-300 dark:border-zinc-700 bg-zinc-100 dark:bg-zinc-800 px-2.5 py-1 text-xs text-zinc-700 dark:text-zinc-300 hover:border-brand-500 hover:text-brand-300 transition-colors"
    >
      + {LABELS[type] ?? type}
    </button>
  {/each}
</div>

<input type="hidden" name="steps" value={stepsJson} />
