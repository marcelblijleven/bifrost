<script lang="ts">
	import type { PageData } from './$types';
	import RunStatusBadge from '$lib/components/RunStatusBadge.svelte';
	import { fmtDateTime } from '$lib/format';

	let { data }: { data: PageData } = $props();

	const providerColors: Record<string, string> = {
		github: 'bg-zinc-200 dark:bg-zinc-700 text-zinc-800 dark:text-zinc-200'
	};
</script>

<svelte:head>
	<title>Applications - Bifrost</title>
</svelte:head>

<div class="p-4 sm:p-8">
	<div class="mb-6 flex items-center justify-between">
		<div>
			<h1 class="text-xl font-semibold text-zinc-900 dark:text-zinc-100">Applications</h1>
			<p class="mt-0.5 text-sm text-zinc-400 dark:text-zinc-500">{data.applications.length} {data.applications.length === 1 ? 'application' : 'applications'}</p>
		</div>
		<a
			href="/applications/new"
			class="rounded-md border border-brand-500 bg-brand-500 dark:bg-brand-500/20 px-4 py-2 text-sm font-medium text-white dark:text-brand-300 hover:bg-brand-600 dark:hover:bg-brand-500/40 hover:text-white transition-colors"
		>
			New application
		</a>
	</div>

	{#if data.applications.length === 0}
		<div class="rounded-xl border border-dashed border-zinc-300 dark:border-zinc-700 p-12 text-center">
			<p class="text-sm text-zinc-400 dark:text-zinc-500">No applications configured yet.</p>
			<a href="/applications/new" class="mt-3 inline-block text-sm text-brand-300 hover:text-brand-300 transition-colors">
				Create your first application
			</a>
		</div>
	{:else}
		<div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
			{#each data.applications as app}
				<a
					href="/applications/{app.ID}"
					class="group rounded-xl border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 p-5 transition hover:border-zinc-300 dark:hover:border-zinc-700 hover:bg-zinc-100/60 dark:hover:bg-zinc-800/60"
				>
					<div class="mb-3 flex items-start justify-between">
						<h2 class="font-semibold text-zinc-900 dark:text-zinc-100 group-hover:text-black dark:group-hover:text-white">{app.Name}</h2>
						<span class="rounded-md px-2 py-0.5 text-xs font-medium {providerColors[app.Provider] ?? 'bg-zinc-200 dark:bg-zinc-700 text-zinc-800 dark:text-zinc-200'}">
							{app.Provider}
						</span>
					</div>

					<p class="font-mono text-xs text-zinc-500 dark:text-zinc-400">
						{app.Owner}/{app.Repo}
					</p>
					<p class="mt-1 text-xs text-zinc-400 dark:text-zinc-500">
						Branch: <span class="text-zinc-500 dark:text-zinc-400">{app.Branch}</span>
					</p>

					{#if app.HeadState === 'blocked'}
						<p class="mt-2 inline-flex items-center gap-1.5 rounded-full border border-rose-500/30 bg-rose-500/10 px-2.5 py-1 text-xs font-medium text-rose-600 dark:text-rose-400">
							<span class="h-1.5 w-1.5 shrink-0 rounded-full bg-rose-500"></span>
							Blocked — needs attention
						</p>
					{/if}

					<div class="mt-4 flex items-center gap-2 border-t border-zinc-200/60 dark:border-zinc-800/60 pt-3">
						{#if app.LastRun}
							<RunStatusBadge status={app.LastRun.Status} />
							{#if app.LastRun.Tag}
								<span class="rounded-md border border-brand-500/30 bg-brand-500/10 px-2 py-0.5 font-mono text-xs text-brand-600 dark:text-brand-300">
									{app.LastRun.Tag}
								</span>
							{/if}
							<span class="ml-auto text-xs text-zinc-400 dark:text-zinc-500">{fmtDateTime(app.LastRun.CreatedAt)}</span>
						{:else}
							<span class="text-xs text-zinc-400 dark:text-zinc-500">No runs yet</span>
						{/if}
					</div>
				</a>
			{/each}
		</div>
	{/if}
</div>
