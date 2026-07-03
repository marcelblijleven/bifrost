import { describe, it, expect, vi, afterEach } from 'vitest'
import { decodeJwt } from './jwt'

function makeToken(payload: Record<string, unknown>): string {
	const header = btoa(JSON.stringify({ alg: 'HS256', typ: 'JWT' })).replace(/=/g, '')
	const body = btoa(JSON.stringify(payload)).replace(/=/g, '')
	return `${header}.${body}.fakesig`
}

const futureExp = Math.floor(Date.now() / 1000) + 3600
const pastExp = Math.floor(Date.now() / 1000) - 1

describe('decodeJwt', () => {
	afterEach(() => {
		vi.restoreAllMocks()
	})

	it('returns user claims for a valid token with future exp', () => {
		const token = makeToken({ user_id: 'abc123', email: 'user@example.com', exp: futureExp })
		const result = decodeJwt(token)
		expect(result).toEqual({ user_id: 'abc123', email: 'user@example.com', is_admin: false })
	})

	it('returns user claims for a valid token without exp', () => {
		const token = makeToken({ user_id: 'abc123', email: 'user@example.com' })
		const result = decodeJwt(token)
		expect(result).toEqual({ user_id: 'abc123', email: 'user@example.com', is_admin: false })
	})

	it('carries the is_admin claim through', () => {
		const token = makeToken({ user_id: 'abc123', email: 'user@example.com', is_admin: true, exp: futureExp })
		const result = decodeJwt(token)
		expect(result).toEqual({ user_id: 'abc123', email: 'user@example.com', is_admin: true })
	})

	it('returns null for an expired token', () => {
		const token = makeToken({ user_id: 'abc123', email: 'user@example.com', exp: pastExp })
		const result = decodeJwt(token)
		expect(result).toBeNull()
	})

	it('returns null when user_id is missing', () => {
		const token = makeToken({ email: 'user@example.com', exp: futureExp })
		const result = decodeJwt(token)
		expect(result).toBeNull()
	})

	it('returns null when email is missing', () => {
		const token = makeToken({ user_id: 'abc123', exp: futureExp })
		const result = decodeJwt(token)
		expect(result).toBeNull()
	})

	it('returns null for a malformed token (not 3 parts)', () => {
		expect(decodeJwt('notavalidtoken')).toBeNull()
		expect(decodeJwt('two.parts')).toBeNull()
		expect(decodeJwt('four.parts.here.extra')).toBeNull()
	})

	it('returns null for invalid base64 in payload', () => {
		const result = decodeJwt('header.!!!invalid!!!.sig')
		expect(result).toBeNull()
	})

	it('returns null for empty string', () => {
		expect(decodeJwt('')).toBeNull()
	})
})
