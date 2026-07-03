export function decodeJwt(token: string): { user_id: string; email: string; is_admin: boolean } | null {
	try {
		const parts = token.split('.')
		if (parts.length !== 3) return null
		const payload = JSON.parse(Buffer.from(parts[1], 'base64').toString('utf-8'))
		if (payload.exp && payload.exp * 1000 < Date.now()) return null
		if (!payload.user_id || !payload.email) return null
		return { user_id: payload.user_id, email: payload.email, is_admin: payload.is_admin === true }
	} catch {
		return null
	}
}
