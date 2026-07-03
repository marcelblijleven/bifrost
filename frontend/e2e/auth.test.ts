import { test, expect } from '@playwright/test'

test.describe('Authentication', () => {
	test('redirects unauthenticated users to login', async ({ page }) => {
		await page.goto('/')
		await expect(page).toHaveURL(/\/login/)
	})

	test('shows login form', async ({ page }) => {
		await page.goto('/login')
		await expect(page.getByLabel('Email')).toBeVisible()
		await expect(page.getByLabel('Password')).toBeVisible()
		await expect(page.getByRole('button', { name: 'Sign in' })).toBeVisible()
	})

	test('shows error on invalid credentials', async ({ page }) => {
		await page.goto('/login')
		await page.getByLabel('Email').fill('wrong@example.com')
		await page.getByLabel('Password').fill('wrongpassword')
		await page.getByRole('button', { name: 'Sign in' }).click()
		await expect(page.getByText('Invalid email or password')).toBeVisible()
	})
})
