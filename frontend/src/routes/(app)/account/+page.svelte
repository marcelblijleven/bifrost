<script lang="ts">
	import type { PageData } from './$types';
	import { api } from '$lib/api';

	let { data }: { data: PageData } = $props();

	let currentPassword = $state('');
	let newPassword = $state('');
	let confirmPassword = $state('');
	let error = $state('');
	let success = $state(false);
	let submitting = $state(false);

	async function changePassword(e: SubmitEvent) {
		e.preventDefault();
		error = '';
		success = false;

		if (!currentPassword || !newPassword) {
			error = 'Current and new password are required';
			return;
		}
		if (newPassword.length < 8) {
			error = 'New password must be at least 8 characters';
			return;
		}
		if (newPassword !== confirmPassword) {
			error = 'New passwords do not match';
			return;
		}

		submitting = true;
		try {
			await api.auth.changePassword(currentPassword, newPassword);
			success = true;
			currentPassword = '';
			newPassword = '';
			confirmPassword = '';
		} catch (err: unknown) {
			error = err instanceof Error ? err.message : 'Failed to change password';
		} finally {
			submitting = false;
		}
	}
</script>

<svelte:head><title>Account - Bifrost</title></svelte:head>

<div class="p-4 sm:p-8 max-w-lg">
	<div class="mb-6">
		<h1 class="text-xl font-semibold text-zinc-900 dark:text-zinc-100">Account</h1>
		<p class="mt-1 text-sm text-zinc-400 dark:text-zinc-500">{data.user?.email}</p>
	</div>

	<div class="rounded-xl border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 p-5">
		<h2 class="mb-4 text-sm font-medium text-zinc-500 dark:text-zinc-400">Change password</h2>

		{#if error}
			<div class="mb-3 rounded-md border border-red-500/20 bg-red-500/10 px-3 py-2 text-xs text-red-600 dark:text-red-400">{error}</div>
		{/if}
		{#if success}
			<div class="mb-3 rounded-md border border-emerald-500/20 bg-emerald-500/10 px-3 py-2 text-xs text-emerald-600 dark:text-emerald-400">Password changed.</div>
		{/if}

		<form onsubmit={changePassword} class="flex flex-col gap-3">
			<div>
				<label for="currentPassword" class="mb-1.5 block text-xs text-zinc-400 dark:text-zinc-500">Current password</label>
				<input id="currentPassword" name="currentPassword" type="password" autocomplete="current-password" required bind:value={currentPassword}
					class="w-full rounded-md border border-zinc-300 dark:border-zinc-700 bg-zinc-50 dark:bg-zinc-800 px-3 py-2 text-sm text-zinc-900 dark:text-zinc-100 placeholder-zinc-400 dark:placeholder-zinc-600 focus:border-brand-500 focus:outline-none" />
			</div>
			<div>
				<label for="newPassword" class="mb-1.5 block text-xs text-zinc-400 dark:text-zinc-500">New password</label>
				<input id="newPassword" name="newPassword" type="password" autocomplete="new-password" required minlength="8" bind:value={newPassword}
					class="w-full rounded-md border border-zinc-300 dark:border-zinc-700 bg-zinc-50 dark:bg-zinc-800 px-3 py-2 text-sm text-zinc-900 dark:text-zinc-100 placeholder-zinc-400 dark:placeholder-zinc-600 focus:border-brand-500 focus:outline-none" />
			</div>
			<div>
				<label for="confirmPassword" class="mb-1.5 block text-xs text-zinc-400 dark:text-zinc-500">Confirm new password</label>
				<input id="confirmPassword" name="confirmPassword" type="password" autocomplete="new-password" required minlength="8" bind:value={confirmPassword}
					class="w-full rounded-md border border-zinc-300 dark:border-zinc-700 bg-zinc-50 dark:bg-zinc-800 px-3 py-2 text-sm text-zinc-900 dark:text-zinc-100 placeholder-zinc-400 dark:placeholder-zinc-600 focus:border-brand-500 focus:outline-none" />
			</div>
			<button type="submit" disabled={submitting}
				class="mt-1 self-start rounded-md bg-brand-600 px-4 py-2 text-sm font-semibold text-white hover:bg-brand-500 transition-colors disabled:opacity-60">
				Change password
			</button>
		</form>
	</div>
</div>
