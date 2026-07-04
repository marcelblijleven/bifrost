<script lang="ts">
	import { goto, invalidateAll } from '$app/navigation';
	import type { PageData } from './$types';
	import { api } from '$lib/api';
	import RunStatusBadge from '$lib/components/RunStatusBadge.svelte';
	import { fmtDateTime, fmtDurationBetween } from '$lib/format';

	let { data }: { data: PageData } = $props();

	const { application, runs, page, limit, status, branch } = $derived(data);

	let accepting = $state(false);
	let acceptError = $state('');

	async function acceptHead(triggerRun: boolean) {
		accepting = true;
		acceptError = '';
		try {
			const res = await api.applications.acceptHead(application.ID, triggerRun);
			if (res.run_id) {
				await goto(`/applications/${application.ID}/runs/${res.run_id}`);
				return;
			}
			await invalidateAll();
		} catch {
			acceptError = 'Failed to accept the current branch head';
		} finally {
			accepting = false;
		}
	}

	let filterStatus = $state(status);
	let filterBranch = $state(branch);

	function gotoPage(p: number) {
		const params = new URLSearchParams();
		if (filterStatus) params.set('status', filterStatus);
		if (filterBranch) params.set('branch', filterBranch);
		params.set('page', String(p));
		goto(`?${params.toString()}`);
	}

	function applyFilters() {
		gotoPage(1);
	}

	function clearFilters() {
		filterStatus = '';
		filterBranch = '';
		goto('?page=1');
	}

	function shortSHA(sha: string) {
		return sha.slice(0, 7);
	}
</script>

<svelte:head>
	<title>{application.Name} - Bifrost</title>
</svelte:head>

<div class="p-4 sm:p-8">
	<!-- Header -->
	<div class="mb-6">
		<div class="mb-4 flex items-center gap-2 text-sm text-zinc-400 dark:text-zinc-500">
			<a href="/applications" class="hover:text-zinc-700 dark:hover:text-zinc-300 transition-colors">Applications</a>
			<span>/</span>
			<span class="text-zinc-700 dark:text-zinc-300">{application.Name}</span>
		</div>
		<div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between sm:gap-4">
			<div class="min-w-0">
				<h1 class="text-xl font-semibold text-zinc-900 dark:text-zinc-100">{application.Name}</h1>
				<p class="mt-0.5 truncate font-mono text-sm text-zinc-400 dark:text-zinc-500">{application.Owner}/{application.Repo}</p>
			</div>
			<a
				href="/applications/{application.ID}/edit"
				class="shrink-0 rounded-md border border-zinc-300 dark:border-zinc-700 px-3 py-1.5 text-xs font-medium text-zinc-700 dark:text-zinc-300 hover:border-zinc-400 dark:hover:border-zinc-500 hover:text-zinc-900 dark:hover:text-white transition-colors"
			>
				Edit
			</a>
		</div>
	</div>

	<!-- Blocked banner: force push / history rewrite detected -->
	{#if application.HeadState === 'blocked'}
		<div class="mb-6 rounded-xl border border-rose-500/30 bg-rose-500/5 p-4 sm:p-5">
			<div class="flex items-start gap-3">
				<svg class="mt-0.5 h-5 w-5 shrink-0 text-rose-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
						d="M12 9v4m0 4h.01M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z" />
				</svg>
				<div class="min-w-0 flex-1">
					<h2 class="text-sm font-semibold text-rose-600 dark:text-rose-400">
						Branch history rewritten — releases paused
					</h2>
					<p class="mt-1 text-sm text-zinc-600 dark:text-zinc-300">{application.BlockedReason}</p>
					{#if application.BlockedAt}
						<p class="mt-1 text-xs text-zinc-400 dark:text-zinc-500">Detected {fmtDateTime(application.BlockedAt)}</p>
					{/if}
					<ol class="mt-3 list-decimal space-y-1 pl-5 text-sm text-zinc-600 dark:text-zinc-300">
						<li>Confirm the rewrite of <span class="font-mono text-xs">{application.Branch}</span> was intentional with whoever pushed it.</li>
						<li>Verify existing release tags still point at commits reachable from the branch.</li>
						<li>Accept the branch's current head below to resume releases from it.</li>
					</ol>
					{#if acceptError}
						<p class="mt-3 text-sm text-rose-600 dark:text-rose-400">{acceptError}</p>
					{/if}
					<div class="mt-4 flex flex-wrap gap-2">
						<button
							type="button"
							onclick={() => acceptHead(false)}
							disabled={accepting}
							class="rounded-md border border-rose-500/40 bg-rose-500/10 px-3 py-1.5 text-xs font-medium text-rose-600 dark:text-rose-400 transition-colors hover:bg-rose-500/20 disabled:pointer-events-none disabled:opacity-40"
						>
							Accept current head
						</button>
						<button
							type="button"
							onclick={() => acceptHead(true)}
							disabled={accepting}
							class="rounded-md border border-rose-500/40 bg-rose-500/10 px-3 py-1.5 text-xs font-medium text-rose-600 dark:text-rose-400 transition-colors hover:bg-rose-500/20 disabled:pointer-events-none disabled:opacity-40"
						>
							Accept &amp; run pipeline
						</button>
					</div>
				</div>
			</div>
		</div>
	{/if}

	<!-- Info chips -->
	<div class="mb-6 flex flex-wrap gap-2">
		<span class="rounded-full border border-zinc-300 dark:border-zinc-700 px-3 py-0.5 text-xs text-zinc-500 dark:text-zinc-400">
			{application.Provider}
		</span>
		<span class="rounded-full border border-zinc-300 dark:border-zinc-700 px-3 py-0.5 text-xs text-zinc-500 dark:text-zinc-400">
			branch: {application.Branch}
		</span>
		{#if application.TriggerType === 'tag'}
			<span class="rounded-full border border-zinc-300 dark:border-zinc-700 px-3 py-0.5 text-xs text-zinc-500 dark:text-zinc-400">
				triggers on tags: {application.TagPattern}
			</span>
		{/if}
		{#if application.TagPrefix}
			<span class="rounded-full border border-zinc-300 dark:border-zinc-700 px-3 py-0.5 text-xs text-zinc-500 dark:text-zinc-400">
				tag prefix: {application.TagPrefix}
			</span>
		{/if}
	</div>

	<!-- Pipeline steps -->
	{#if application.PipelineSteps?.length}
		<div class="mb-8">
			<h2 class="mb-3 text-sm font-medium text-zinc-500 dark:text-zinc-400">Pipeline</h2>
			<div class="flex flex-wrap items-center gap-2">
				{#each application.PipelineSteps as step, i}
					{#if i > 0}
						<svg class="h-3 w-3 text-zinc-400 dark:text-zinc-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7" />
						</svg>
					{/if}
					<span class="rounded-lg border border-zinc-300 dark:border-zinc-700 bg-zinc-100/60 dark:bg-zinc-800/60 px-3 py-1 text-xs font-medium text-zinc-700 dark:text-zinc-300">
						{step.type}
					</span>
				{/each}
			</div>
		</div>
	{/if}

	<!-- Runs table -->
	<div>
		<h2 class="mb-3 text-sm font-medium text-zinc-500 dark:text-zinc-400">Pipeline runs</h2>

		<!-- Filters -->
		<div class="mb-3 flex flex-wrap items-center gap-2">
			<div class="relative">
				<svg
					class="pointer-events-none absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-zinc-400 dark:text-zinc-600"
					fill="none"
					stroke="currentColor"
					viewBox="0 0 24 24"
				>
					<path
						stroke-linecap="round"
						stroke-linejoin="round"
						stroke-width="2"
						d="m21 21-5.197-5.197m0 0A7.5 7.5 0 1 0 5.196 5.196a7.5 7.5 0 0 0 10.607 10.607Z"
					/>
				</svg>
				<input
					bind:value={filterBranch}
					placeholder="Filter by branch…"
					onchange={applyFilters}
					class="w-44 rounded-md border border-zinc-300 dark:border-zinc-700 bg-zinc-100 dark:bg-zinc-800 py-1.5 pl-8 pr-3 text-xs text-zinc-700 dark:text-zinc-300 placeholder-zinc-400 dark:placeholder-zinc-600 focus:border-brand-500 focus:outline-none"
				/>
			</div>
			<select
				bind:value={filterStatus}
				onchange={applyFilters}
				class="rounded-md border border-zinc-300 dark:border-zinc-700 bg-zinc-100 dark:bg-zinc-800 px-3 py-1.5 text-xs text-zinc-700 dark:text-zinc-300 focus:border-brand-500 focus:outline-none"
			>
				<option value="">All statuses</option>
				<option value="pending">Pending</option>
				<option value="running">Running</option>
				<option value="success">Success</option>
				<option value="failed">Failed</option>
				<option value="cancelled">Cancelled</option>
				<option value="superseded">Superseded</option>
				<option value="skipped">Skipped</option>
				<option value="blocked">Blocked</option>
			</select>
			{#if filterStatus || filterBranch}
				<button onclick={clearFilters} class="text-xs text-zinc-400 dark:text-zinc-500 hover:text-zinc-700 dark:hover:text-zinc-300 transition">
					Clear
				</button>
			{/if}
		</div>

		{#if runs.length === 0 && page === 1}
			<div class="rounded-xl border border-dashed border-zinc-300 dark:border-zinc-700 p-8 text-center">
				<p class="text-sm text-zinc-400 dark:text-zinc-500">No pipeline runs yet.</p>
			</div>
		{:else if runs.length === 0}
			<div class="rounded-xl border border-dashed border-zinc-300 dark:border-zinc-700 p-8 text-center">
				<p class="text-sm text-zinc-400 dark:text-zinc-500">No more runs on this page.</p>
			</div>
		{:else}
			<div class="overflow-x-auto rounded-xl border border-zinc-200 dark:border-zinc-800">
				<table class="w-full min-w-[700px] text-sm">
					<thead>
						<tr class="border-b border-zinc-200 dark:border-zinc-800 bg-zinc-50/80 dark:bg-zinc-900/80">
							<th class="px-4 py-2.5 text-left text-[11px] font-semibold uppercase tracking-wider text-zinc-500 dark:text-zinc-400">Commit</th>
							<th class="px-4 py-2.5 text-left text-[11px] font-semibold uppercase tracking-wider text-zinc-500 dark:text-zinc-400">Branch</th>
							<th class="px-4 py-2.5 text-left text-[11px] font-semibold uppercase tracking-wider text-zinc-500 dark:text-zinc-400">Tag</th>
							<th class="px-4 py-2.5 text-left text-[11px] font-semibold uppercase tracking-wider text-zinc-500 dark:text-zinc-400">Status</th>
							<th class="px-4 py-2.5 text-left text-[11px] font-semibold uppercase tracking-wider text-zinc-500 dark:text-zinc-400">Duration</th>
							<th class="px-4 py-2.5 text-left text-[11px] font-semibold uppercase tracking-wider text-zinc-500 dark:text-zinc-400">Started</th>
						</tr>
					</thead>
					<tbody class="divide-y divide-zinc-200/60 dark:divide-zinc-800/60">
						{#each runs as run}
							<tr class="bg-white dark:bg-zinc-900 transition-colors hover:bg-zinc-50 dark:hover:bg-zinc-800/40">
								<td class="px-4 py-3">
									<a href="/applications/{application.ID}/runs/{run.ID}" class="group/link inline-flex items-start gap-2">
										<svg class="mt-0.5 h-3.5 w-3.5 shrink-0 text-zinc-300 dark:text-zinc-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
											<circle cx="12" cy="12" r="3.25" stroke-width="2" />
											<path stroke-linecap="round" stroke-width="2" d="M3.5 12h5.25M15.25 12h5.25" />
										</svg>
										<span>
											<span
												class="rounded-md border border-zinc-200 dark:border-zinc-700 bg-zinc-50 dark:bg-zinc-800/60 px-1.5 py-0.5 font-mono text-xs text-zinc-600 dark:text-zinc-300 transition-colors group-hover/link:border-brand-500/50 group-hover/link:text-brand-500 dark:group-hover/link:text-brand-300"
											>
												{shortSHA(run.CommitSHA)}
											</span>
											{#if run.CommitMessage}
												<p class="mt-1 max-w-[180px] truncate text-xs text-zinc-400 dark:text-zinc-500">{run.CommitMessage}</p>
											{/if}
										</span>
									</a>
								</td>
								<td class="px-4 py-3">
									<span class="rounded-md border border-zinc-200 dark:border-zinc-700 bg-zinc-50 dark:bg-zinc-800/60 px-2 py-0.5 font-mono text-xs text-zinc-600 dark:text-zinc-300">
										{run.Branch}
									</span>
								</td>
								<td class="px-4 py-3">
									{#if run.Tag}
										<span class="rounded-md border border-brand-500/30 bg-brand-500/10 px-2 py-0.5 font-mono text-xs text-brand-600 dark:text-brand-300">
											{run.Tag}
										</span>
									{:else}
										<span class="text-xs text-zinc-300 dark:text-zinc-700">-</span>
									{/if}
								</td>
								<td class="px-4 py-3">
									<RunStatusBadge status={run.Status} />
								</td>
								<td class="px-4 py-3 tabular-nums text-xs text-zinc-500 dark:text-zinc-400">{fmtDurationBetween(run.StartedAt, run.CompletedAt)}</td>
								<td class="px-4 py-3 text-xs text-zinc-400 dark:text-zinc-500">
									{run.StartedAt ? fmtDateTime(run.StartedAt) : '-'}
								</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>

			<!-- Pagination -->
			{#if runs.length === limit || page > 1}
				<div class="mt-4 flex items-center justify-between">
					<button
						onclick={() => gotoPage(page - 1)}
						disabled={page <= 1}
						class="rounded-md border border-zinc-300 dark:border-zinc-700 px-3 py-1.5 text-xs font-medium text-zinc-500 dark:text-zinc-400 transition-colors hover:border-zinc-400 dark:hover:border-zinc-500 hover:text-zinc-800 dark:hover:text-zinc-200 disabled:pointer-events-none disabled:opacity-40"
					>
						Previous
					</button>
					<span class="text-xs text-zinc-400 dark:text-zinc-500">Page {page}</span>
					<button
						onclick={() => gotoPage(page + 1)}
						disabled={runs.length < limit}
						class="rounded-md border border-zinc-300 dark:border-zinc-700 px-3 py-1.5 text-xs font-medium text-zinc-500 dark:text-zinc-400 transition-colors hover:border-zinc-400 dark:hover:border-zinc-500 hover:text-zinc-800 dark:hover:text-zinc-200 disabled:pointer-events-none disabled:opacity-40"
					>
						Next
					</button>
				</div>
			{/if}
		{/if}
	</div>
</div>
