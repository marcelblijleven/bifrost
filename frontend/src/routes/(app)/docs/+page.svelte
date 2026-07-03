<script lang="ts">
	import type { PageData } from './$types';
	let { data }: { data: PageData } = $props();
</script>

<div class="mx-auto max-w-3xl px-6 py-10">
	<h1 class="mb-1 text-2xl font-bold text-zinc-900 dark:text-zinc-100">Documentation</h1>
	<p class="mb-8 text-sm text-zinc-500 dark:text-zinc-400">Reference guides for configuring and operating Bifrost.</p>

	<div class="divide-y divide-zinc-200 dark:divide-zinc-800 rounded-xl border border-zinc-200 dark:border-zinc-800">
		{#each data.docs as doc}
			<a
				href="/docs/{doc.slug}"
				class="flex items-center justify-between px-5 py-4 transition first:rounded-t-xl last:rounded-b-xl hover:bg-zinc-100/50 dark:hover:bg-zinc-800/50"
			>
				<div>
					<p class="text-sm font-medium text-zinc-900 dark:text-zinc-100">{doc.title}</p>
					<p class="mt-0.5 text-xs text-zinc-500 dark:text-zinc-400">{doc.description}</p>
				</div>
				<svg class="h-4 w-4 shrink-0 text-zinc-400 dark:text-zinc-600" fill="none" stroke="currentColor" stroke-width="1.5" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" d="M9 5l7 7-7 7" />
				</svg>
			</a>
		{/each}
	</div>
</div>
