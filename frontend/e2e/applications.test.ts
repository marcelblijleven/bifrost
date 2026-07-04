import { test, expect, type Page } from '@playwright/test'

async function loginAs(page: Page, email: string, password: string) {
	const res = await page.request.post('http://localhost:8080/api/auth/login', {
		data: { email, password },
	})
	const { token } = await res.json()
	await page.context().addCookies([
		{
			name: 'token',
			value: token,
			domain: 'localhost',
			path: '/',
			httpOnly: true,
			sameSite: 'Strict',
		},
	])
}

test.describe('Applications', () => {
	test.beforeEach(async ({ page }) => {
		try {
			await loginAs(
				page,
				process.env.TEST_EMAIL ?? 'test@example.com',
				process.env.TEST_PASSWORD ?? 'testpassword',
			)
		} catch {
			test.skip()
		}
	})

	test('shows applications list', async ({ page }) => {
		await page.goto('/applications')
		await expect(page.getByRole('heading', { name: 'Applications' })).toBeVisible()
	})

	test('has new application button', async ({ page }) => {
		await page.goto('/applications')
		await expect(page.getByRole('link', { name: 'New application' })).toBeVisible()
	})

	test('new application form has required fields', async ({ page }) => {
		await page.goto('/applications/new')
		await expect(page.getByLabel('Name')).toBeVisible()
		await expect(page.getByLabel('Owner')).toBeVisible()
		await expect(page.getByLabel('Repository')).toBeVisible()
		await expect(page.getByLabel('Branch')).toBeVisible()
		await expect(page.getByLabel('Webhook secret')).toBeVisible()
	})

	test('generate button fills webhook secret field', async ({ page }) => {
		await page.goto('/applications/new')
		const secretInput = page.getByLabel('Webhook secret')
		await expect(secretInput).toHaveValue('')
		// Retry the click: a click that lands before hydration is a no-op.
		await expect(async () => {
			await page.getByRole('button', { name: 'Generate' }).click()
			expect(await secretInput.inputValue()).not.toBe('')
		}).toPass()
	})
})
