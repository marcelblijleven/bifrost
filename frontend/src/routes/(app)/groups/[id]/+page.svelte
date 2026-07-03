<script lang="ts">
	import { enhance } from '$app/forms';
	import type { PageData, ActionData } from './$types';

	let { data, form }: { data: PageData; form: ActionData } = $props();
	const { group, members, users } = $derived(data);

	const nonMembers = $derived(users.filter((u) => !members.some((m) => m.ID === u.ID)));

	let renaming = $state(false);
</script>

<svelte:head><title>{group.Name} - Bifrost</title></svelte:head>

<div class="p-4 sm:p-8 max-w-2xl">
	<div class="mb-4 flex items-center gap-2 text-sm text-zinc-400 dark:text-zinc-500">
		<a href="/users" class="hover:text-zinc-700 dark:hover:text-zinc-300 transition-colors">Team</a>
		<span>/</span>
		<a href="/users?tab=groups" class="hover:text-zinc-700 dark:hover:text-zinc-300 transition-colors">Groups</a>
		<span>/</span>
		<span class="text-zinc-700 dark:text-zinc-300">{group.Name}</span>
	</div>

	{#if renaming}
		{#if form?.error}
			<div class="mb-2 rounded-md border border-red-500/20 bg-red-500/10 px-3 py-2 text-xs text-red-600 dark:text-red-400">{form.error}</div>
		{/if}
		<form method="POST" action="?/rename" use:enhance={() => {
			return async ({ result, update }) => {
				if (result.type !== 'failure') renaming = false;
				await update();
			};
		}} class="mb-6 flex items-center gap-2">
			<input
				name="name"
				type="text"
				value={group.Name}
				required
				class="rounded-md border border-zinc-300 dark:border-zinc-700 bg-zinc-50 dark:bg-zinc-800 px-3 py-1.5 text-xl font-semibold text-zinc-900 dark:text-zinc-100 focus:border-brand-500 focus:outline-none"
			/>
			<button type="submit"
				class="rounded-md border border-brand-500 bg-brand-500 dark:bg-brand-500/20 px-3 py-1.5 text-xs font-medium text-white dark:text-brand-300 hover:bg-brand-600 dark:hover:bg-brand-500/40 hover:text-white transition-colors">
				Save
			</button>
			<button type="button" onclick={() => renaming = false}
				class="text-xs text-zinc-400 dark:text-zinc-600 hover:text-zinc-700 dark:hover:text-zinc-300 transition-colors">
				Cancel
			</button>
		</form>
	{:else}
		<div class="mb-6 flex items-center gap-2">
			<h1 class="text-xl font-semibold text-zinc-900 dark:text-zinc-100">{group.Name}</h1>
			<button type="button" onclick={() => renaming = true}
				class="text-xs text-zinc-400 dark:text-zinc-600 hover:text-zinc-700 dark:hover:text-zinc-300 transition-colors">
				Rename
			</button>
		</div>
	{/if}

	{#if nonMembers.length > 0}
		<div class="mb-6 rounded-xl border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 p-5">
			<h2 class="mb-3 text-sm font-medium text-zinc-500 dark:text-zinc-400">Add member</h2>
			<form method="POST" action="?/addMember" use:enhance class="flex gap-3">
				<select name="userId"
					class="flex-1 rounded-md border border-zinc-300 dark:border-zinc-700 bg-zinc-50 dark:bg-zinc-800 px-3 py-2 text-sm text-zinc-900 dark:text-zinc-100 focus:border-brand-500 focus:outline-none">
					{#each nonMembers as u (u.ID)}
						<option value={u.ID}>{u.Email}</option>
					{/each}
				</select>
				<button type="submit"
					class="rounded-md border border-brand-500 bg-brand-500 dark:bg-brand-500/20 px-4 py-2 text-sm font-medium text-white dark:text-brand-300 hover:bg-brand-600 dark:hover:bg-brand-500/40 hover:text-white transition-colors">
					Add
				</button>
			</form>
		</div>
	{/if}

	<h2 class="mb-3 text-sm font-medium text-zinc-500 dark:text-zinc-400">Members ({members.length})</h2>
	{#if members.length === 0}
		<div class="rounded-xl border border-dashed border-zinc-300 dark:border-zinc-700 p-6 text-center">
			<p class="text-sm text-zinc-400 dark:text-zinc-500">No members yet.</p>
		</div>
	{:else}
		<div class="space-y-2">
			{#each members as member (member.ID)}
				<div class="flex items-center justify-between rounded-xl border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 px-4 py-3">
					<span class="text-sm text-zinc-800 dark:text-zinc-200">{member.Email}</span>
					<form method="POST" action="?/removeMember" use:enhance>
						<input type="hidden" name="userId" value={member.ID} />
						<button type="submit" class="text-xs text-zinc-400 dark:text-zinc-600 transition-colors hover:text-red-400">Remove</button>
					</form>
				</div>
			{/each}
		</div>
	{/if}
</div>
