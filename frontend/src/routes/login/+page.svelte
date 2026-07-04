<script lang="ts">
	import { goto } from '$app/navigation';
	import { api, ApiError } from '$lib/api';

	let email = $state('');
	let password = $state('');
	let error = $state('');
	let submitting = $state(false);

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		submitting = true;
		error = '';
		try {
			await api.auth.login(email, password);
			await goto('/');
		} catch (err: unknown) {
			error =
				err instanceof ApiError && err.status === 429
					? 'Too many failed attempts. Try again in a few minutes.'
					: 'Invalid email or password';
		} finally {
			submitting = false;
		}
	}
</script>

<svelte:head>
	<title>Bifrost - Sign in</title>
</svelte:head>

<div class="flex min-h-screen items-center justify-center bg-zinc-50 dark:bg-zinc-950 px-4">
	<div class="w-full max-w-sm">
		<div class="mb-8 text-center">
			<div class="mb-3 inline-flex items-center gap-3">
				<img src="/logo.svg" alt="Bifrost" class="h-10 w-10" />
				<h1 class="text-2xl font-bold text-zinc-900 dark:text-white">Bifrost</h1>
			</div>
			<p class="text-sm text-zinc-500 dark:text-zinc-400">Release orchestration</p>
		</div>

		<div class="rounded-2xl border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 p-8">
			{#if error}
				<div class="mb-4 rounded-lg border border-red-800 bg-red-950/50 px-4 py-3 text-sm text-red-400">
					{error}
				</div>
			{/if}

			<form onsubmit={submit} class="space-y-4">
				<div>
					<label for="email" class="mb-1.5 block text-sm font-medium text-zinc-700 dark:text-zinc-300">Email</label>
					<input
						id="email"
						name="email"
						type="email"
						autocomplete="email"
						required
						bind:value={email}
						class="w-full rounded-lg border border-zinc-300 dark:border-zinc-700 bg-zinc-50 dark:bg-zinc-800 px-3 py-2 text-sm text-zinc-900 dark:text-zinc-100 placeholder-zinc-400 dark:placeholder-zinc-500 outline-none focus:border-brand-300 focus:ring-1 focus:ring-brand-300"
						placeholder="you@example.com"
					/>
				</div>

				<div>
					<label for="password" class="mb-1.5 block text-sm font-medium text-zinc-700 dark:text-zinc-300">Password</label>
					<input
						id="password"
						name="password"
						type="password"
						autocomplete="current-password"
						required
						bind:value={password}
						class="w-full rounded-lg border border-zinc-300 dark:border-zinc-700 bg-zinc-50 dark:bg-zinc-800 px-3 py-2 text-sm text-zinc-900 dark:text-zinc-100 placeholder-zinc-400 dark:placeholder-zinc-500 outline-none focus:border-brand-300 focus:ring-1 focus:ring-brand-300"
						placeholder="••••••••"
					/>
				</div>

				<button
					type="submit"
					disabled={submitting}
					class="w-full rounded-lg bg-brand-500 px-4 py-2 text-sm font-semibold text-white transition hover:bg-brand-300 focus:outline-none focus:ring-2 focus:ring-brand-300 focus:ring-offset-2 focus:ring-offset-white dark:focus:ring-offset-zinc-900 disabled:opacity-60"
				>
					Sign in
				</button>
			</form>
		</div>
	</div>
</div>
