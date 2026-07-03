import { fail, redirect, error, isRedirect } from '@sveltejs/kit'
import type { PageServerLoad, Actions } from './$types'
import { createApi } from '$lib/api'
import { parseNotifications, parseSkipConditions, parseTrigger } from '$lib/server/appForm'

export const load: PageServerLoad = async ({ params, locals, fetch }) => {
  const api = createApi(fetch, locals.token)
  try {
    const [application, assignedGroups, allGroups, providers] = await Promise.all([
      api.applications.get(params.id),
      api.applications.listGroups(params.id).catch(() => []),
      locals.user?.is_admin ? api.groups.list().catch(() => []) : Promise.resolve([]),
      api.providers.list().then((r) => r.providers).catch((): string[] => []),
    ])
    return { application, assignedGroups, allGroups, providers }
  } catch (e) {
    if (isRedirect(e)) throw e
    error(404, 'Application not found')
  }
}

export const actions: Actions = {
  update: async ({ request, params, locals, fetch }) => {
    const data = await request.formData()

    const name = (data.get('name') as string)?.trim()
    const provider = (data.get('provider') as string)?.trim()
    const owner = (data.get('owner') as string)?.trim()
    const repo = (data.get('repo') as string)?.trim()
    const branch = (data.get('branch') as string)?.trim() || 'main'
    let webhookSecret = (data.get('webhook_secret') as string)?.trim()
    const stepsRaw = (data.get('steps') as string)?.trim()

    if (!name || !provider || !owner || !repo) {
      return fail(400, {
        error: 'Name, provider, owner, repo, and branch are required',
        values: { name, provider, owner, repo, branch },
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

    try {
      const api = createApi(fetch, locals.token)
      await api.applications.update(params.id, {
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
    } catch (e: unknown) {
      return fail(500, {
        error: e instanceof Error ? e.message : 'Failed to update application',
        values: { name, provider, owner, repo, branch, webhookSecret },
      })
    }
    redirect(303, `/applications/${params.id}`)
  },

  delete: async ({ params, locals, fetch }) => {
    const api = createApi(fetch, locals.token)
    try {
      await api.applications.delete(params.id)
    } catch {
      return fail(500, { error: 'Failed to delete application' })
    }
    redirect(303, '/applications')
  },

  grantGroup: async ({ params, locals, fetch, request }) => {
    const data = await request.formData()
    const groupId = (data.get('addGroupId') as string)?.trim()
    if (!groupId) return fail(400, { groupError: 'Group is required' })
    const api = createApi(fetch, locals.token)
    try {
      await api.applications.grantGroup(params.id, groupId)
    } catch (e: unknown) {
      return fail(422, { groupError: e instanceof Error ? e.message : 'Failed to grant access' })
    }
  },

  revokeGroup: async ({ params, locals, fetch, request }) => {
    const data = await request.formData()
    const groupId = (data.get('revokeGroupId') as string)?.trim()
    if (!groupId) return fail(400, { groupError: 'Group is required' })
    const api = createApi(fetch, locals.token)
    try {
      await api.applications.revokeGroup(params.id, groupId)
    } catch (e: unknown) {
      return fail(422, { groupError: e instanceof Error ? e.message : 'Failed to revoke access' })
    }
  },

  installWebhook: async ({ params, locals, fetch }) => {
    const api = createApi(fetch, locals.token)
    try {
      const result = await api.applications.installWebhook(params.id)
      return { webhookInstalled: true, webhookURL: result.webhook_url }
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : 'Failed to install webhook'
      return fail(422, { webhookError: msg })
    }
  },
}
