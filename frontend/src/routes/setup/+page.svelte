<script lang="ts">
	import { enhance } from '$app/forms';
	import type { ActionData } from './$types';

	let { form }: { form: ActionData } = $props();
</script>

<svelte:head>
	<title>Setup - Bifrost</title>
</svelte:head>

<div class="flex min-h-screen items-center justify-center bg-zinc-50 dark:bg-zinc-950 px-4">
	<div class="w-full max-w-md">
		<!-- Logo / wordmark -->
		<div class="mb-8 text-center">
			<div class="mb-3 inline-flex h-12 w-12 items-center justify-center rounded-xl bg-brand-500">
				<svg class="h-7 w-7 text-white" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" d="M13 10V3L4 14h7v7l9-11h-7z" />
				</svg>
			</div>
			<h1 class="text-2xl font-bold text-zinc-900 dark:text-white">Welcome to Bifrost</h1>
			<p class="mt-1.5 text-sm text-zinc-500 dark:text-zinc-400">Create your admin account to get started</p>
		</div>

		<div class="rounded-2xl border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 p-8">
			<form method="POST" use:enhance>
				<div class="space-y-5">
					<div>
						<label for="email" class="mb-1.5 block text-sm font-medium text-zinc-700 dark:text-zinc-300">
							Email address
						</label>
						<input
							id="email"
							name="email"
							type="email"
							required
							autocomplete="email"
							placeholder="you@example.com"
							class="w-full rounded-lg border border-zinc-300 dark:border-zinc-700 bg-zinc-50 dark:bg-zinc-800 px-3 py-2.5 text-sm text-zinc-900 dark:text-white placeholder-zinc-400 dark:placeholder-zinc-500 outline-none transition focus:border-brand-300 focus:ring-1 focus:ring-brand-300"
						/>
					</div>

					<div>
						<label for="password" class="mb-1.5 block text-sm font-medium text-zinc-700 dark:text-zinc-300">
							Password
						</label>
						<input
							id="password"
							name="password"
							type="password"
							required
							autocomplete="new-password"
							placeholder="At least 8 characters"
							class="w-full rounded-lg border border-zinc-300 dark:border-zinc-700 bg-zinc-50 dark:bg-zinc-800 px-3 py-2.5 text-sm text-zinc-900 dark:text-white placeholder-zinc-400 dark:placeholder-zinc-500 outline-none transition focus:border-brand-300 focus:ring-1 focus:ring-brand-300"
						/>
					</div>

					<div>
						<label for="confirm" class="mb-1.5 block text-sm font-medium text-zinc-700 dark:text-zinc-300">
							Confirm password
						</label>
						<input
							id="confirm"
							name="confirm"
							type="password"
							required
							autocomplete="new-password"
							placeholder="Repeat your password"
							class="w-full rounded-lg border border-zinc-300 dark:border-zinc-700 bg-zinc-50 dark:bg-zinc-800 px-3 py-2.5 text-sm text-zinc-900 dark:text-white placeholder-zinc-400 dark:placeholder-zinc-500 outline-none transition focus:border-brand-300 focus:ring-1 focus:ring-brand-300"
						/>
					</div>

					{#if form?.error}
						<p class="rounded-lg border border-red-900 bg-red-950/40 px-3 py-2.5 text-sm text-red-400">
							{form.error}
						</p>
					{/if}

					<button
						type="submit"
						class="w-full rounded-lg bg-brand-500 py-2.5 text-sm font-semibold text-white transition hover:bg-brand-300 active:bg-brand-600"
					>
						Create account
					</button>
				</div>
			</form>
		</div>

		<p class="mt-6 text-center text-xs text-zinc-400 dark:text-zinc-600">
			Additional users can be invited after setup is complete.
		</p>
	</div>
</div>
