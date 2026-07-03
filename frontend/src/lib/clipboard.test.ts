import { describe, it, expect, vi, beforeEach } from 'vitest'
import { copyToClipboard } from './clipboard'

describe('copyToClipboard', () => {
	beforeEach(() => {
		vi.restoreAllMocks()
	})

	it('uses navigator.clipboard.writeText when available', async () => {
		const writeText = vi.fn().mockResolvedValue(undefined)
		Object.defineProperty(navigator, 'clipboard', {
			value: { writeText },
			configurable: true,
		})

		await copyToClipboard('hello')

		expect(writeText).toHaveBeenCalledOnce()
		expect(writeText).toHaveBeenCalledWith('hello')
	})

	it('falls back to execCommand when clipboard.writeText throws', async () => {
		const writeText = vi.fn().mockRejectedValue(new Error('not allowed'))
		Object.defineProperty(navigator, 'clipboard', {
			value: { writeText },
			configurable: true,
		})

		const execCommand = vi.fn().mockReturnValue(true)
		document.execCommand = execCommand

		await copyToClipboard('fallback text')

		expect(writeText).toHaveBeenCalledOnce()
		expect(execCommand).toHaveBeenCalledWith('copy')
	})

	it('uses execCommand when clipboard API is not available', async () => {
		Object.defineProperty(navigator, 'clipboard', {
			value: undefined,
			configurable: true,
		})

		const execCommand = vi.fn().mockReturnValue(true)
		document.execCommand = execCommand

		await copyToClipboard('no clipboard api')

		expect(execCommand).toHaveBeenCalledWith('copy')
	})
})
