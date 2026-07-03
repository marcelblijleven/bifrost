import { test, expect } from '@playwright/test'

test.describe('Run detail page', () => {
	test('redirects to login when not authenticated', async ({ page }) => {
		await page.goto('/applications/some-id/runs/some-run-id')
		await expect(page).toHaveURL(/\/login/)
	})
})
