<script lang="ts">
	import type { PageData } from './$types';
	import RunStatusBadge from '$lib/components/RunStatusBadge.svelte';
	import { fmtDateTime, fmtDate, fmtDuration, fmtDurationBetween } from '$lib/format';

	let { data }: { data: PageData } = $props();
	const { stats } = $derived(data);

	const successRate = $derived(
		stats.total_runs > 0 ? Math.round((stats.succeeded_runs / stats.total_runs) * 100) : 0
	);

	function shortSHA(sha: string) {
		return sha.slice(0, 7);
	}

	const chartDays = $derived(stats.runs_by_day.slice(-14));
	const chartMax = $derived(Math.max(...chartDays.map((d) => d.total), 1));

	function barHeight(value: number, max: number): number {
		return Math.round((value / max) * 100);
	}

	const donutR = 36;
	const donutCircumference = $derived(2 * Math.PI * donutR);
	const donutSucceeded = $derived(
		stats.total_runs > 0
			? (stats.succeeded_runs / stats.total_runs) * donutCircumference
			: 0
	);
	const donutFailed = $derived(
		stats.total_runs > 0
			? (stats.failed_runs / stats.total_runs) * donutCircumference
			: 0
	);
	const donutOther = $derived(donutCircumference - donutSucceeded - donutFailed);
</script>

<svelte:head>
	<title>Dashboard - Bifrost</title>
</svelte:head>

<div class="p-4 sm:p-8">
	<div class="mb-6">
		<h1 class="text-xl font-semibold text-zinc-900 dark:text-zinc-100">Dashboard</h1>
		<p class="mt-0.5 text-sm text-zinc-400 dark:text-zinc-500">Last 30 days</p>
	</div>

	<!-- Pending actions -->
	{#if stats.pending_actions.length > 0}
		<div class="mb-8 rounded-xl border border-amber-500/40 dark:border-amber-900/50 bg-amber-50 dark:bg-amber-950/20 p-5">
			<div class="mb-3 flex items-center gap-2">
				<svg class="h-4 w-4 text-amber-600 dark:text-amber-400" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" d="M12 9v3.75m-9.303 3.376c-.866 1.5.217 3.374 1.948 3.374h14.71c1.73 0 2.813-1.874 1.948-3.374L13.949 3.378c-.866-1.5-3.032-1.5-3.898 0L2.697 16.126ZM12 15.75h.007v.008H12v-.008Z" />
				</svg>
				<h2 class="text-sm font-medium text-amber-700 dark:text-amber-400">
					{stats.pending_actions.length} pending action{stats.pending_actions.length !== 1 ? 's' : ''}
				</h2>
			</div>
			<div class="divide-y divide-amber-500/20 dark:divide-amber-900/30">
				{#each stats.pending_actions as action}
					<a
						href="/applications/{action.application_id}/runs/{action.run_id}"
						class="flex items-center justify-between py-3 transition hover:bg-amber-100 dark:hover:bg-amber-950/30 rounded-md px-2 -mx-2"
					>
						<div>
							<p class="text-sm font-medium text-zinc-800 dark:text-zinc-200">{action.application_name}</p>
							<p class="mt-0.5 text-xs text-zinc-500 dark:text-zinc-400">{action.message}</p>
						</div>
						<span class="ml-4 shrink-0 rounded-md px-2 py-0.5 text-xs font-medium {action.type === 'approval' ? 'bg-amber-100 dark:bg-amber-950 text-amber-700 dark:text-amber-400' : 'bg-zinc-100 dark:bg-zinc-800 text-zinc-500 dark:text-zinc-400'}">
							{action.type === 'approval' ? 'Needs approval' : 'Queued'}
						</span>
					</a>
				{/each}
			</div>
		</div>
	{/if}

	<!-- Stat cards -->
	<div class="mb-8 grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
		<div class="rounded-xl border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 p-5">
			<p class="text-xs font-medium uppercase tracking-wider text-zinc-400 dark:text-zinc-500">Total runs</p>
			<p class="mt-2 text-3xl font-bold text-zinc-900 dark:text-zinc-100">{stats.total_runs}</p>
		</div>
		<div class="rounded-xl border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 p-5">
			<p class="text-xs font-medium uppercase tracking-wider text-zinc-400 dark:text-zinc-500">Success rate</p>
			<p class="mt-2 text-3xl font-bold text-emerald-400">{successRate}%</p>
		</div>
		<div class="rounded-xl border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 p-5">
			<p class="text-xs font-medium uppercase tracking-wider text-zinc-400 dark:text-zinc-500">Failed runs</p>
			<p class="mt-2 text-3xl font-bold text-red-400">{stats.failed_runs}</p>
		</div>
		<div class="rounded-xl border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 p-5">
			<p class="text-xs font-medium uppercase tracking-wider text-zinc-400 dark:text-zinc-500">Avg duration</p>
			<p class="mt-2 text-3xl font-bold text-zinc-900 dark:text-zinc-100">
				{stats.avg_duration_seconds > 0 ? fmtDuration(stats.avg_duration_seconds) : '-'}
			</p>
		</div>
	</div>

	<!-- Charts row -->
	<div class="mb-8 grid gap-4 lg:grid-cols-3">
		<!-- Bar chart: runs over time -->
		<div class="col-span-2 rounded-xl border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 p-5">
			<p class="mb-4 text-sm font-medium text-zinc-700 dark:text-zinc-300">Runs - last 14 days</p>
			{#if chartDays.every((d) => d.total === 0)}
				<div class="flex min-h-[10rem] items-center justify-center">
					<p class="text-sm text-zinc-400 dark:text-zinc-500">No runs yet</p>
				</div>
			{:else}
				<div class="flex h-40 gap-1">
					{#each chartDays as day}
						<div class="group relative flex-1">
							<!-- tooltip -->
							<div
								class="pointer-events-none absolute bottom-full z-10 mb-1 hidden w-max rounded bg-zinc-100 dark:bg-zinc-800 px-2 py-1 text-xs text-zinc-800 dark:text-zinc-200 group-hover:block"
							>
								{fmtDate(day.date)}: {day.total} run{day.total !== 1 ? 's' : ''}
							</div>
							<!-- stacked bar anchored to the bottom -->
							{#if day.total > 0}
								<div
									class="absolute bottom-0 left-0 right-0 flex flex-col-reverse overflow-hidden rounded-sm"
									style="height: {barHeight(day.total, chartMax)}%"
								>
									{#if day.succeeded > 0}
										<div class="w-full bg-emerald-500 opacity-80" style="flex: {day.succeeded}"></div>
									{/if}
									{#if day.failed > 0}
										<div class="w-full bg-red-500 opacity-80" style="flex: {day.failed}"></div>
									{/if}
									{#if day.total - day.succeeded - day.failed > 0}
										<div class="w-full bg-zinc-400 dark:bg-zinc-600 opacity-80" style="flex: {day.total - day.succeeded - day.failed}"></div>
									{/if}
								</div>
							{/if}
						</div>
					{/each}
				</div>
				<!-- x-axis labels -->
				<div class="mt-1 flex justify-between text-xs text-zinc-400 dark:text-zinc-500">
					<span>{chartDays[0] ? fmtDate(chartDays[0].date) : ''}</span>
					<span>{chartDays[6] ? fmtDate(chartDays[6].date) : ''}</span>
					<span>{chartDays[chartDays.length - 1] ? fmtDate(chartDays[chartDays.length - 1].date) : ''}</span>
				</div>
				<!-- legend -->
				<div class="mt-3 flex gap-4 border-t border-zinc-200/60 dark:border-zinc-800/60 pt-3 text-xs text-zinc-400 dark:text-zinc-500">
					<span class="flex items-center gap-1.5"><span class="h-2 w-2 rounded-sm bg-emerald-500"></span>Succeeded</span>
					<span class="flex items-center gap-1.5"><span class="h-2 w-2 rounded-sm bg-red-500"></span>Failed</span>
					<span class="flex items-center gap-1.5"><span class="h-2 w-2 rounded-sm bg-zinc-400 dark:bg-zinc-600"></span>Other</span>
				</div>
			{/if}
		</div>

		<!-- Donut chart: status breakdown -->
		<div class="rounded-xl border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 p-5">
			<p class="mb-4 text-sm font-medium text-zinc-700 dark:text-zinc-300">Status breakdown</p>
			{#if stats.total_runs === 0}
				<p class="py-8 text-center text-sm text-zinc-400 dark:text-zinc-600">No runs yet</p>
			{:else}
				<div class="flex items-center gap-6 lg:flex-col lg:items-center">
					<svg viewBox="0 0 100 100" class="h-36 w-36 shrink-0 -rotate-90 lg:h-32 lg:w-32">
						<circle cx="50" cy="50" r={donutR} fill="none" stroke="currentColor" stroke-width="14" class="text-zinc-200 dark:text-zinc-800" />
						<circle
							cx="50" cy="50" r={donutR} fill="none" stroke="#10b981" stroke-width="14"
							stroke-dasharray="{donutSucceeded} {donutCircumference - donutSucceeded}"
							stroke-dashoffset="0"
						/>
						<circle
							cx="50" cy="50" r={donutR} fill="none" stroke="#ef4444" stroke-width="14"
							stroke-dasharray="{donutFailed} {donutCircumference - donutFailed}"
							stroke-dashoffset="{-donutSucceeded}"
						/>
						{#if donutOther > 0.5}
							<circle
								cx="50" cy="50" r={donutR} fill="none" stroke="#52525b" stroke-width="14"
								stroke-dasharray="{donutOther} {donutCircumference - donutOther}"
								stroke-dashoffset="{-(donutSucceeded + donutFailed)}"
							/>
						{/if}
					</svg>
					<div class="space-y-1.5 text-sm">
						<div class="flex items-center justify-between gap-6">
							<span class="flex items-center gap-1.5 text-zinc-500 dark:text-zinc-400"><span class="h-2.5 w-2.5 rounded-full bg-emerald-500"></span>Succeeded</span>
							<span class="font-medium text-zinc-800 dark:text-zinc-200">{stats.succeeded_runs}</span>
						</div>
						<div class="flex items-center justify-between gap-6">
							<span class="flex items-center gap-1.5 text-zinc-500 dark:text-zinc-400"><span class="h-2.5 w-2.5 rounded-full bg-red-500"></span>Failed</span>
							<span class="font-medium text-zinc-800 dark:text-zinc-200">{stats.failed_runs}</span>
						</div>
						<div class="flex items-center justify-between gap-6">
							<span class="flex items-center gap-1.5 text-zinc-500 dark:text-zinc-400"><span class="h-2.5 w-2.5 rounded-full bg-zinc-400 dark:bg-zinc-600"></span>Other</span>
							<span class="font-medium text-zinc-800 dark:text-zinc-200">{stats.total_runs - stats.succeeded_runs - stats.failed_runs}</span>
						</div>
					</div>
				</div>
			{/if}
		</div>
	</div>

	<!-- Recent runs -->
	<div class="rounded-xl border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900">
		<div class="border-b border-zinc-200 dark:border-zinc-800 px-5 py-4">
			<h2 class="text-sm font-medium text-zinc-700 dark:text-zinc-300">Recent runs</h2>
		</div>
		{#if stats.recent_runs.length === 0}
			<p class="py-10 text-center text-sm text-zinc-400 dark:text-zinc-600">No runs yet</p>
		{:else}
			<div class="divide-y divide-zinc-200 dark:divide-zinc-800">
				{#each stats.recent_runs as run}
					<a
						href="/applications/{run.ApplicationID}/runs/{run.ID}"
						class="flex items-center justify-between px-5 py-3.5 transition hover:bg-zinc-100/50 dark:hover:bg-zinc-800/50"
					>
						<div class="min-w-0">
							<div class="flex items-center gap-2">
								<span class="truncate text-sm font-medium text-zinc-800 dark:text-zinc-200">{run.application_name}</span>
								<span class="shrink-0 rounded-md border border-zinc-200 dark:border-zinc-700 bg-zinc-50 dark:bg-zinc-800/60 px-1.5 py-0.5 font-mono text-xs text-zinc-500 dark:text-zinc-400">
									{shortSHA(run.CommitSHA)}
								</span>
							</div>
							<p class="mt-0.5 text-xs text-zinc-400 dark:text-zinc-500">{run.Branch} · {fmtDateTime(run.CreatedAt)}</p>
						</div>
						<div class="ml-4 flex shrink-0 items-center gap-3">
							{#if run.StartedAt && run.CompletedAt}
								<span class="tabular-nums text-xs text-zinc-400 dark:text-zinc-500">
									{fmtDurationBetween(run.StartedAt, run.CompletedAt)}
								</span>
							{/if}
							<RunStatusBadge status={run.Status} />
						</div>
					</a>
				{/each}
			</div>
		{/if}
	</div>
</div>
