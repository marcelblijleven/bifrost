import { error, isRedirect } from '@sveltejs/kit'
import type { PageLoad } from './$types'
import { createApi } from '$lib/api'

export const load: PageLoad = async ({ params, fetch, parent }) => {
  const api = createApi(fetch)
  const { user } = await parent()
  try {
    const [application, assignedGroups, allGroups, providers] = await Promise.all([
      api.applications.get(params.id),
      api.applications.listGroups(params.id).catch(() => []),
      user?.is_admin ? api.groups.list().catch(() => []) : Promise.resolve([]),
      api.providers.list().then((r) => r.providers).catch((): string[] => []),
    ])
    return { application, assignedGroups, allGroups, providers }
  } catch (e) {
    if (isRedirect(e)) throw e
    error(404, 'Application not found')
  }
}
