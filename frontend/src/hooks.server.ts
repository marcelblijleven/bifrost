import type { Handle } from '@sveltejs/kit'
import { decodeJwt } from '$lib/jwt'

export const handle: Handle = async ({ event, resolve }) => {
  if (event.url.pathname === '/logout') {
    return new Response(null, {
      status: 302,
      headers: {
        Location: '/login',
        'Set-Cookie': 'token=; Path=/; Max-Age=0; HttpOnly; SameSite=Strict'
      }
    })
  }

  const token = event.cookies.get('token')
  if (token) {
    const user = decodeJwt(token)
    if (user) {
      event.locals.token = token
      event.locals.user = user
    } else {
      event.cookies.delete('token', { path: '/' })
    }
  }
  return resolve(event)
}
