<script lang="ts">
	// Translucent tinted badge: text + wash background + matching-tint border,
	// with a solid dot echoing the same state so it reads at a glance.
	// Single source of truth for run status colours across all pages.
	const COLORS: Record<string, string> = {
		pending: 'text-zinc-500 dark:text-zinc-400 bg-zinc-500/10 border-zinc-500/20 dark:border-zinc-500/30',
		running: 'text-blue-600 dark:text-blue-400 bg-blue-500/10 border-blue-500/20 dark:border-blue-500/30',
		success: 'text-emerald-600 dark:text-emerald-400 bg-emerald-500/10 border-emerald-500/20 dark:border-emerald-500/30',
		failed: 'text-red-600 dark:text-red-400 bg-red-500/10 border-red-500/20 dark:border-red-500/30',
		cancelled: 'text-zinc-500 dark:text-zinc-400 bg-zinc-500/10 border-zinc-500/20 dark:border-zinc-500/30',
		superseded: 'text-amber-600 dark:text-amber-400 bg-amber-500/10 border-amber-500/20 dark:border-amber-500/30',
		skipped: 'text-zinc-400 dark:text-zinc-500 bg-transparent border-dashed border-zinc-300 dark:border-zinc-700',
		blocked: 'text-rose-600 dark:text-rose-400 bg-rose-500/10 border-rose-500/20 dark:border-rose-500/30'
	};

	const DOTS: Record<string, string> = {
		pending: 'bg-zinc-400',
		running: 'bg-blue-500',
		success: 'bg-emerald-500',
		failed: 'bg-red-500',
		cancelled: 'bg-zinc-400',
		superseded: 'bg-amber-500',
		skipped: 'bg-zinc-300 dark:bg-zinc-600',
		blocked: 'bg-rose-500'
	};

	let { status }: { status: string } = $props();
</script>

<span class="inline-flex items-center gap-1.5 rounded-full border px-2.5 py-1 text-xs font-medium {COLORS[status] ?? COLORS.pending}">
	<span class="h-1.5 w-1.5 shrink-0 rounded-full {DOTS[status] ?? DOTS.pending}"></span>
	{status}
</span>
