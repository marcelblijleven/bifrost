import type { PageLoad } from './$types'
import { createApi } from '$lib/api'

export const load: PageLoad = async ({ fetch }) => {
  const api = createApi(fetch)
  const providers = await api.providers
    .list()
    .then((r) => r.providers)
    .catch((): string[] => [])
  return { providers }
}
