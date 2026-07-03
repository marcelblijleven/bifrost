import type { RequestHandler } from '@sveltejs/kit'

const API_URL = process.env.API_URL ?? 'http://localhost:8080'

// Proxy the Go backend's SSE stream so the client can subscribe without
// CORS concerns (everything is served from the same SvelteKit origin).
export const GET: RequestHandler = async ({ params, locals, request }) => {
	const upstream = await fetch(`${API_URL}/runs/${params.runId}/events`, {
		headers: locals.token ? { Authorization: `Bearer ${locals.token}` } : {},
		signal: request.signal,
	})

	return new Response(upstream.body, {
		headers: {
			'Content-Type': 'text/event-stream',
			'Cache-Control': 'no-cache',
			'Connection': 'keep-alive',
			'X-Accel-Buffering': 'no',
		},
	})
}
