<script lang="ts">
	import { enhance } from '$app/forms';
	import { invalidateAll } from '$app/navigation';
	import type { PageData, ActionData } from './$types';
	import type { StepResult, ApprovalRequest } from '$lib/types';
	import RunStatusBadge from '$lib/components/RunStatusBadge.svelte';
	import { fmtDateTime, fmtDurationBetween } from '$lib/format';

	let { data, form }: { data: PageData; form: ActionData } = $props();

	const { run, steps, approvals, appId, application } = $derived(data);

	// Step index whose override-reason form is open, or null.
	let overrideFor = $state<number | null>(null);

	function githubActionsUrl(externalRunID: number): string {
		if (!application) return '';
		return `https://github.com/${application.Owner}/${application.Repo}/actions/runs/${externalRunID}`;
	}

	function parseOutput(output: string): Record<string, unknown> {
		if (!output) return {};
		try { return JSON.parse(output); } catch { return {}; }
	}

	function workflowConclusion(output: string): string {
		return String(parseOutput(output).conclusion ?? '');
	}

	function conclusionColor(conclusion: string): string {
		return {
			success: 'text-emerald-600 dark:text-emerald-400 bg-emerald-500/10 border-emerald-500/20 dark:border-emerald-500/30',
			skipped: 'text-zinc-500 dark:text-zinc-400 bg-zinc-500/10 border-zinc-500/20 dark:border-zinc-500/30',
			failure: 'text-red-600 dark:text-red-400 bg-red-500/10 border-red-500/20 dark:border-red-500/30',
			cancelled: 'text-zinc-500 dark:text-zinc-400 bg-zinc-500/10 border-zinc-500/20 dark:border-zinc-500/30',
			timed_out: 'text-amber-600 dark:text-amber-400 bg-amber-500/10 border-amber-500/20 dark:border-amber-500/30'
		}[conclusion] ?? 'text-zinc-500 dark:text-zinc-400 bg-zinc-500/10 border-zinc-500/20 dark:border-zinc-500/30';
	}

	const TERMINAL = new Set(['success', 'failed', 'cancelled', 'superseded', 'skipped']);

	$effect(() => {
		if (TERMINAL.has(run.Status)) return;

		const es = new EventSource(`/applications/${appId}/runs/${run.ID}/events`);

		// Refetch on (re)connect so nothing is missed while disconnected. Errors
		// are left to EventSource's built-in retry — closing here would silently
		// freeze the page for the rest of the run.
		es.addEventListener('open', () => invalidateAll());
		es.addEventListener('update', () => invalidateAll());

		return () => es.close();
	});

	function stepStatusIcon(status: StepResult['Status']) {
		return {
			pending: { icon: 'M12 6v6l4 2', color: 'text-zinc-400 dark:text-zinc-500' },
			running: { icon: 'M12 6v6l4 2', color: 'text-blue-400 animate-pulse' },
			success: { icon: 'M4.5 12.75l6 6 9-13.5', color: 'text-emerald-400' },
			failed: { icon: 'M6 18 18 6M6 6l12 12', color: 'text-red-400' },
			skipped: { icon: 'M6 12h12', color: 'text-zinc-400 dark:text-zinc-600' },
			cancelled: { icon: 'M6 12h12', color: 'text-amber-500' },
			overridden: { icon: 'M4.5 12.75l6 6 9-13.5', color: 'text-amber-500' }
		}[status] ?? { icon: 'M12 6v6l4 2', color: 'text-zinc-400 dark:text-zinc-500' };
	}

	function approvalForStep(stepIndex: number): ApprovalRequest | undefined {
		return approvals.find((a) => a.StepIndex === stepIndex);
	}

	function shortSHA(sha: string) {
		return sha.slice(0, 7);
	}

	function stepOutputEntries(step: StepResult): Array<[string, unknown]> | null {
		const obj: Record<string, unknown> = {};
		if (step.Output) {
			try { Object.assign(obj, JSON.parse(step.Output)); } catch { /* ignore */ }
		}
		if (step.ErrorMessage) {
			obj.error = step.ErrorMessage;
		}
		const entries = Object.entries(obj);
		return entries.length > 0 ? entries : null;
	}

	function isUrl(val: unknown): val is string {
		return typeof val === 'string' && (val.startsWith('http://') || val.startsWith('https://'));
	}
</script>

<svelte:head>
	<title>Run {run.ID.slice(0, 8)} - Bifrost</title>
</svelte:head>

<div class="p-4 sm:p-8">
	<!-- Breadcrumb -->
	<div class="mb-4 flex items-center gap-2 text-sm text-zinc-400 dark:text-zinc-500">
		<a href="/applications" class="hover:text-zinc-700 dark:hover:text-zinc-300 transition-colors">Applications</a>
		<span>/</span>
		<a href="/applications/{appId}" class="hover:text-zinc-700 dark:hover:text-zinc-300 transition-colors">{application?.Name ?? run.ApplicationID.slice(0, 8)}</a>
		<span>/</span>
		<span class="text-zinc-700 dark:text-zinc-300">Run {run.ID.slice(0, 8)}</span>
	</div>

	<!-- Run header -->
	<div class="mb-8 rounded-xl border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 p-5">
		<div class="flex items-start justify-between gap-4">
			<div class="min-w-0">
				<div class="mb-2 flex items-center gap-2.5 flex-wrap">
					<RunStatusBadge status={run.Status} />
					<span class="font-mono text-xs text-zinc-400 dark:text-zinc-500">Run {run.ID.slice(0, 8)}</span>
					{#if run.Tag}
						<span class="rounded-md border border-brand-500/30 bg-brand-500/10 px-2 py-0.5 font-mono text-xs text-brand-600 dark:text-brand-300">{run.Tag}</span>
					{/if}
				</div>
				<p class="font-mono text-sm text-zinc-700 dark:text-zinc-300 mb-1">
					<span class="text-brand-300">{shortSHA(run.CommitSHA)}</span>
					<span class="text-zinc-400 dark:text-zinc-500"> on </span>
					<span class="text-zinc-800 dark:text-zinc-200">{run.Branch}</span>
				</p>
				{#if run.CommitMessage}
					<p class="text-xs text-zinc-400 dark:text-zinc-500 truncate">{run.CommitMessage}</p>
				{/if}
				{#if run.TriggeredBy}
					<p class="mt-1.5 text-xs text-zinc-400 dark:text-zinc-600">triggered by <span class="text-zinc-500 dark:text-zinc-400">{run.TriggeredBy}</span></p>
				{/if}
			</div>
			<div class="flex shrink-0 items-start gap-3">
				{#if run.Status === 'pending' || run.Status === 'running'}
					<form method="POST" action="?/cancel" use:enhance>
						<button
							type="submit"
							class="rounded-md border border-zinc-300 dark:border-zinc-700 px-3 py-1.5 text-xs font-medium text-zinc-500 dark:text-zinc-400 transition-colors hover:border-red-700/60 hover:text-red-400"
						>
							Cancel run
						</button>
					</form>
				{/if}
				<div class="text-right text-xs text-zinc-400 dark:text-zinc-500">
					<p>Duration: {fmtDurationBetween(run.StartedAt, run.CompletedAt)}</p>
					{#if run.StartedAt}
						<p class="mt-0.5">Started {fmtDateTime(run.StartedAt)}</p>
					{/if}
				</div>
			</div>
		</div>
	</div>

	<!-- Steps -->
	<h2 class="mb-3 text-sm font-medium text-zinc-500 dark:text-zinc-400">Steps</h2>
	<div class="space-y-3">
		{#each steps as step}
			{@const si = stepStatusIcon(step.Status)}
			{@const pendingApproval = approvalForStep(step.StepIndex)}
			{@const entries = stepOutputEntries(step)}

			<div class="rounded-xl border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 p-4">
				<div class="flex items-start gap-3">
					<!-- Status icon -->
					<div class="mt-0.5 flex h-5 w-5 shrink-0 items-center justify-center">
						<svg class="h-4 w-4 {si.color}" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" d={si.icon} />
						</svg>
					</div>

					<div class="flex-1 min-w-0">
						<div class="flex items-center gap-2">
							<span class="text-sm font-medium text-zinc-800 dark:text-zinc-200">{step.StepName}</span>
							<span class="text-xs text-zinc-400 dark:text-zinc-500">#{step.StepIndex}</span>
							<span class="ml-auto text-xs text-zinc-400 dark:text-zinc-500">
								{fmtDurationBetween(step.StartedAt, step.CompletedAt)}
							</span>
						</div>

						{#if step.ExternalRunID}
							{@const conclusion = workflowConclusion(step.Output)}
							<div class="mt-2 flex items-center gap-2">
								<a
									href={githubActionsUrl(step.ExternalRunID)}
									target="_blank"
									rel="noopener noreferrer"
									class="inline-flex items-center gap-1.5 rounded-md border border-zinc-300 dark:border-zinc-700 bg-zinc-100/60 dark:bg-zinc-800/60 px-2.5 py-1 text-xs text-zinc-700 dark:text-zinc-300 transition-colors hover:border-zinc-400 dark:hover:border-zinc-600 hover:text-zinc-900 dark:hover:text-white"
								>
									<svg class="h-3.5 w-3.5" fill="currentColor" viewBox="0 0 24 24">
										<path d="M12 2C6.477 2 2 6.477 2 12c0 4.418 2.865 8.166 6.839 9.489.5.092.682-.217.682-.482 0-.237-.009-.868-.014-1.703-2.782.604-3.369-1.341-3.369-1.341-.454-1.154-1.11-1.462-1.11-1.462-.908-.62.069-.608.069-.608 1.003.07 1.531 1.03 1.531 1.03.892 1.529 2.341 1.087 2.91.832.092-.647.35-1.088.636-1.338-2.22-.253-4.555-1.11-4.555-4.943 0-1.091.39-1.984 1.029-2.683-.103-.253-.446-1.27.098-2.647 0 0 .84-.268 2.75 1.026A9.578 9.578 0 0112 6.836a9.59 9.59 0 012.504.337c1.909-1.294 2.747-1.026 2.747-1.026.546 1.377.202 2.394.1 2.647.64.699 1.028 1.592 1.028 2.683 0 3.842-2.339 4.687-4.566 4.935.359.309.678.919.678 1.852 0 1.336-.012 2.415-.012 2.743 0 .267.18.578.688.48C19.138 20.162 22 16.416 22 12c0-5.523-4.477-10-10-10z"/>
									</svg>
									Actions run #{step.ExternalRunID}
									{#if conclusion}
										<span class="rounded-full border px-1.5 py-0.5 text-[10px] font-medium {conclusionColor(conclusion)}">
											{conclusion}
										</span>
									{:else if step.Status === 'running'}
										<span class="rounded-full border border-blue-500/20 dark:border-blue-500/30 bg-blue-500/10 px-1.5 py-0.5 text-[10px] font-medium text-blue-600 dark:text-blue-400">
											running
										</span>
									{:else}
										<span class="rounded-full border border-zinc-500/20 dark:border-zinc-500/30 bg-zinc-500/10 px-1.5 py-0.5 text-[10px] font-medium text-zinc-500 dark:text-zinc-400">
											dispatched
										</span>
									{/if}
								</a>
							</div>
						{/if}

						{#if entries}
							<dl class="mt-2 rounded-lg bg-zinc-50 dark:bg-zinc-950 px-3 py-2 text-xs {step.ErrorMessage ? 'border border-red-900/50' : ''}">
								{#each entries as [key, val]}
									<div class="flex gap-2 py-0.5">
										<dt class="shrink-0 font-mono text-zinc-400 dark:text-zinc-500">{key}:</dt>
										<dd class="min-w-0 break-all font-mono text-zinc-700 dark:text-zinc-300">
											{#if isUrl(val)}
												<a href={String(val)} target="_blank" rel="noopener noreferrer"
												   class="text-brand-300 underline hover:text-brand-300">
													{String(val)}
												</a>
											{:else if key === 'error'}
												<span class="text-red-400">{String(val)}</span>
											{:else}
												{String(val)}
											{/if}
										</dd>
									</div>
								{/each}
							</dl>
						{/if}

						{#if step.Status === 'overridden'}
							<div class="mt-3 rounded-lg border border-amber-500/40 dark:border-amber-800/60 bg-amber-50 dark:bg-amber-950/30 px-3 py-2">
								<p class="text-xs text-amber-700 dark:text-amber-400">
									Failure overridden by <span class="font-medium">{step.OverriddenBy}</span>
									{#if step.OverriddenAt}on {fmtDateTime(step.OverriddenAt)}{/if}
								</p>
								{#if step.OverrideReason}
									<p class="mt-1 text-xs text-zinc-600 dark:text-zinc-300">“{step.OverrideReason}”</p>
								{/if}
							</div>
						{/if}

						{#if (step.Status === 'failed' || step.Status === 'cancelled') && (run.Status === 'failed' || run.Status === 'cancelled')}
							<div class="mt-2 flex justify-end gap-2">
								{#if step.Status === 'failed' && run.Status === 'failed'}
									<button
										type="button"
										onclick={() => (overrideFor = overrideFor === step.StepIndex ? null : step.StepIndex)}
										class="rounded-md border border-amber-600/50 px-3 py-1.5 text-xs font-medium text-amber-600 dark:text-amber-400 transition-colors hover:bg-amber-500/10"
									>
										Override & continue
									</button>
								{/if}
								<form method="POST" action="?/retry" use:enhance>
									<input type="hidden" name="stepIndex" value={step.StepIndex} />
									<button
										type="submit"
										class="rounded-md bg-brand-600 px-3 py-1.5 text-xs font-semibold text-white transition-colors hover:bg-brand-500"
									>
										Retry
									</button>
								</form>
							</div>

							{#if overrideFor === step.StepIndex}
								<form method="POST" action="?/override" use:enhance class="mt-2 rounded-lg border border-amber-500/40 dark:border-amber-800/60 bg-amber-50 dark:bg-amber-950/30 p-3">
									<input type="hidden" name="stepIndex" value={step.StepIndex} />
									<label class="mb-1 block text-xs font-medium text-amber-700 dark:text-amber-400" for="override-reason-{step.StepIndex}">
										Why is it safe to continue past this failure?
									</label>
									<textarea
										id="override-reason-{step.StepIndex}"
										name="reason"
										rows="2"
										required
										placeholder="e.g. deploy succeeded; only the post-deploy smoke test flaked"
										class="w-full rounded border border-zinc-300 dark:border-zinc-600 bg-white dark:bg-zinc-900 px-2 py-1.5 text-xs text-zinc-900 dark:text-white placeholder-zinc-400 dark:placeholder-zinc-500 focus:outline-none focus:ring-1 focus:ring-amber-500"
									></textarea>
									<p class="mt-1 text-xs text-zinc-400 dark:text-zinc-500">
										The pipeline resumes from the next step. Your name and this reason are recorded on the run for the audit trail.
									</p>
									{#if form?.error}
										<p class="mt-1 text-xs text-red-500">{form.error}</p>
									{/if}
									<div class="mt-2 flex justify-end gap-2">
										<button
											type="button"
											onclick={() => (overrideFor = null)}
											class="rounded-md border border-zinc-300 dark:border-zinc-700 px-3 py-1.5 text-xs font-medium text-zinc-500 dark:text-zinc-400 transition-colors hover:text-zinc-800 dark:hover:text-zinc-200"
										>
											Cancel
										</button>
										<button
											type="submit"
											class="rounded-md bg-amber-600 px-3 py-1.5 text-xs font-semibold text-white transition-colors hover:bg-amber-500"
										>
											Override & continue
										</button>
									</div>
								</form>
							{/if}
						{/if}

						<!-- Approval block -->
						{#if pendingApproval}
							{#if pendingApproval.Status === 'pending'}
								<div class="mt-3 rounded-lg border border-amber-500/40 dark:border-amber-800/60 bg-amber-100 dark:bg-amber-950/30 p-3">
									<p class="mb-2 text-xs font-medium text-amber-700 dark:text-amber-400">Waiting for approval</p>
									{#if pendingApproval.Message}
										<p class="mb-3 text-xs text-zinc-500 dark:text-zinc-400">{pendingApproval.Message}</p>
									{/if}
									<div class="flex gap-2">
										<form method="POST" action="?/approve" use:enhance>
											<input type="hidden" name="stepIndex" value={step.StepIndex} />
											<button
												type="submit"
												class="rounded-md bg-emerald-700 px-4 py-2 text-sm font-semibold text-white transition-colors hover:bg-emerald-600"
											>
												Approve
											</button>
										</form>
										<form method="POST" action="?/reject" use:enhance>
											<input type="hidden" name="stepIndex" value={step.StepIndex} />
											<button
												type="submit"
												class="rounded-md bg-red-800 px-4 py-2 text-sm font-semibold text-white transition-colors hover:bg-red-700"
											>
												Reject
											</button>
										</form>
									</div>
								</div>
							{:else if pendingApproval.Status === 'superseded'}
								<div class="mt-3 rounded-lg border border-amber-500/40 dark:border-amber-800/40 bg-amber-50 dark:bg-amber-950/20 px-3 py-2">
									<p class="text-xs text-amber-600 dark:text-amber-500">
										Superseded by a newer run
										{#if pendingApproval.SupersededBy}
											:
											<a
												href="/applications/{appId}/runs/{pendingApproval.SupersededBy}"
												class="underline hover:text-amber-500 dark:hover:text-amber-300 transition-colors"
											>
												{pendingApproval.SupersededBy.slice(0, 8)}
											</a>
										{/if}
									</p>
								</div>
							{:else if pendingApproval.Status === 'approved' || pendingApproval.Status === 'rejected'}
								<div class="mt-3 rounded-lg border px-3 py-2 {pendingApproval.Status === 'approved'
									? 'border-emerald-500/30 dark:border-emerald-800/40 bg-emerald-50 dark:bg-emerald-950/20'
									: 'border-red-500/30 dark:border-red-800/40 bg-red-50 dark:bg-red-950/20'}">
									<p class="text-xs {pendingApproval.Status === 'approved' ? 'text-emerald-700 dark:text-emerald-400' : 'text-red-700 dark:text-red-400'}">
										{pendingApproval.Status === 'approved' ? 'Approved' : 'Rejected'}
										{#if pendingApproval.ResolvedBy}
											by <span class="font-medium">{pendingApproval.ResolvedBy}</span>
										{/if}
										{#if pendingApproval.ResolvedAt}
											on {fmtDateTime(pendingApproval.ResolvedAt)}
										{/if}
									</p>
								</div>
							{/if}
						{/if}
					</div>
				</div>
			</div>
		{/each}
	</div>
</div>
