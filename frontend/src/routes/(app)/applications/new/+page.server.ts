import { fail, redirect } from '@sveltejs/kit'
import type { PageServerLoad, Actions } from './$types'
import { createApi } from '$lib/api'
import { parseNotifications, parseSkipConditions, parseTrigger } from '$lib/server/appForm'

export const load: PageServerLoad = async ({ locals, fetch }) => {
  const api = createApi(fetch, locals.token)
  const providers = await api.providers
    .list()
    .then((r) => r.providers)
    .catch((): string[] => [])
  return { providers }
}

export const actions: Actions = {
  default: async ({ request, locals, fetch }) => {
    const data = await request.formData()

    const name = (data.get('name') as string)?.trim()
    const provider = (data.get('provider') as string)?.trim()
    const owner = (data.get('owner') as string)?.trim()
    const repo = (data.get('repo') as string)?.trim()
    const branch = (data.get('branch') as string)?.trim() || 'main'
    const webhookSecret = (data.get('webhook_secret') as string)?.trim()
    const stepsRaw = (data.get('steps') as string)?.trim()

    if (!name || !provider || !owner || !repo || !webhookSecret) {
      return fail(400, {
        error: 'Name, provider, owner, repo, and webhook secret are required',
        values: { name, provider, owner, repo, branch, webhookSecret },
      })
    }

    let pipelineSteps: { type: string; config?: Record<string, unknown> }[] = []
    if (stepsRaw) {
      try {
        pipelineSteps = JSON.parse(stepsRaw)
      } catch {
        return fail(400, {
          error: 'Pipeline steps must be valid JSON',
          values: { name, provider, owner, repo, branch, webhookSecret },
        })
      }
    }

    let appId: string
    try {
      const api = createApi(fetch, locals.token)
      const app = await api.applications.create({
        Name: name,
        Provider: provider,
        Owner: owner,
        Repo: repo,
        Branch: branch,
        WebhookSecret: webhookSecret,
        PipelineSteps: pipelineSteps,
        Notifications: parseNotifications(data),
        SkipConditions: parseSkipConditions(data),
        ...parseTrigger(data),
      })
      appId = app.ID
    } catch (e: unknown) {
      return fail(500, {
        error: e instanceof Error ? e.message : 'Failed to create application',
        values: { name, provider, owner, repo, branch, webhookSecret },
      })
    }
    redirect(303, `/applications/${appId}`)
  },
}
