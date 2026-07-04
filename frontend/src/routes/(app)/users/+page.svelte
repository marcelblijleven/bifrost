<script lang="ts">
	import { invalidateAll, replaceState } from '$app/navigation';
	import { page } from '$app/stores';
	import type { PageData } from './$types';
	import type { User, Group } from '$lib/types';
	import { api } from '$lib/api';
	import { fmtDateOnly } from '$lib/format';

	let { data }: { data: PageData } = $props();
	const { users, groups, user: me } = $derived(data);

	let tab = $state<'users' | 'groups'>(
		$page.url.searchParams.get('tab') === 'groups' ? 'groups' : 'users'
	);
	let showUserForm = $state(false);
	let showGroupForm = $state(false);
	let resetPasswordFor = $state<string | null>(null);
	let error = $state('');
	let errorTab = $state<'users' | 'groups'>('users');

	// Keep the tab in the URL so refresh and links (e.g. the group detail
	// breadcrumb) land on the right one.
	function switchTab(t: 'users' | 'groups') {
		tab = t;
		replaceState(t === 'groups' ? '?tab=groups' : '?tab=users', {});
	}

	function showError(msg: string, t: 'users' | 'groups') {
		error = msg;
		errorTab = t;
		tab = t;
		if (t === 'groups') showGroupForm = true;
		else showUserForm = true;
	}

	async function createUser(e: SubmitEvent) {
		e.preventDefault();
		error = '';
		const form = e.currentTarget as HTMLFormElement;
		const fd = new FormData(form);
		const email = fd.get('email')?.toString().trim() ?? '';
		const password = fd.get('password')?.toString() ?? '';
		if (!email || !password) {
			showError('Email and password are required', 'users');
			return;
		}
		try {
			await api.users.create(email, password);
			form.reset();
			await invalidateAll();
		} catch (err: unknown) {
			showError(err instanceof Error ? err.message : 'Failed to create user', 'users');
		}
	}

	async function setAdmin(user: User) {
		error = '';
		try {
			await api.users.setAdmin(user.ID, !user.IsAdmin);
			await invalidateAll();
		} catch (err: unknown) {
			showError(err instanceof Error ? err.message : 'Failed to update admin rights', 'users');
		}
	}

	async function deleteUser(user: User) {
		if (!confirm(`Delete user "${user.Email}"?`)) return;
		error = '';
		try {
			await api.users.delete(user.ID);
			await invalidateAll();
		} catch (err: unknown) {
			showError(err instanceof Error ? err.message : 'Failed to delete user', 'users');
		}
	}

	async function resetPassword(e: SubmitEvent, userId: string) {
		e.preventDefault();
		error = '';
		const fd = new FormData(e.currentTarget as HTMLFormElement);
		const newPassword = fd.get('newPassword')?.toString() ?? '';
		if (!newPassword || newPassword.length < 8) {
			showError('Password must be at least 8 characters', 'users');
			return;
		}
		try {
			await api.users.resetPassword(userId, newPassword);
			resetPasswordFor = null;
		} catch (err: unknown) {
			showError(err instanceof Error ? err.message : 'Failed to reset password', 'users');
		}
	}

	async function createGroup(e: SubmitEvent) {
		e.preventDefault();
		error = '';
		const form = e.currentTarget as HTMLFormElement;
		const name = new FormData(form).get('name')?.toString().trim() ?? '';
		if (!name) {
			showError('Name is required', 'groups');
			return;
		}
		try {
			await api.groups.create(name);
			form.reset();
			await invalidateAll();
		} catch {
			showError('Failed to create group', 'groups');
		}
	}

	async function deleteGroup(group: Group) {
		if (!confirm(`Delete group "${group.Name}"?`)) return;
		error = '';
		try {
			await api.groups.delete(group.ID);
			await invalidateAll();
		} catch {
			showError('Failed to delete group', 'groups');
		}
	}
</script>

<svelte:head><title>Team - Bifrost</title></svelte:head>

<div class="p-4 sm:p-8 max-w-3xl">
	<div class="mb-6">
		<h1 class="text-xl font-semibold text-zinc-900 dark:text-zinc-100">Team</h1>
	</div>

	<!-- Tabs -->
	<div class="mb-6 flex border-b border-zinc-200 dark:border-zinc-800">
		<button
			onclick={() => switchTab('users')}
			class="px-4 py-2 text-sm font-medium transition-colors border-b-2 -mb-px {tab === 'users' ? 'border-brand-500 text-zinc-900 dark:text-zinc-100' : 'border-transparent text-zinc-400 dark:text-zinc-500 hover:text-zinc-700 dark:hover:text-zinc-300'}"
		>
			Users
		</button>
		<button
			onclick={() => switchTab('groups')}
			class="px-4 py-2 text-sm font-medium transition-colors border-b-2 -mb-px {tab === 'groups' ? 'border-brand-500 text-zinc-900 dark:text-zinc-100' : 'border-transparent text-zinc-400 dark:text-zinc-500 hover:text-zinc-700 dark:hover:text-zinc-300'}"
		>
			Groups
		</button>
	</div>

	<!-- Users tab -->
	{#if tab === 'users'}
		<div class="mb-4 flex items-center justify-between">
			<p class="text-sm text-zinc-400 dark:text-zinc-500">{users.length} {users.length === 1 ? 'user' : 'users'}</p>
			<button
				onclick={() => showUserForm = !showUserForm}
				class="rounded-md border border-brand-500 bg-brand-500 dark:bg-brand-500/20 px-3 py-1.5 text-xs font-medium text-white dark:text-brand-300 hover:bg-brand-600 dark:hover:bg-brand-500/40 hover:text-white transition-colors"
			>
				{showUserForm ? 'Cancel' : 'New user'}
			</button>
		</div>

		{#if showUserForm}
			<div class="mb-4 rounded-xl border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 p-5">
				<h2 class="mb-4 text-sm font-medium text-zinc-500 dark:text-zinc-400">New user</h2>
				{#if error && errorTab === 'users'}
					<div class="mb-3 rounded-md border border-red-500/20 bg-red-500/10 px-3 py-2 text-xs text-red-600 dark:text-red-400">{error}</div>
				{/if}
				<form onsubmit={createUser} class="flex flex-col gap-3 sm:flex-row sm:items-end">
					<div class="flex-1">
						<label for="email" class="mb-1.5 block text-xs text-zinc-400 dark:text-zinc-500">Email</label>
						<input id="email" name="email" type="email" required
							class="w-full rounded-md border border-zinc-300 dark:border-zinc-700 bg-zinc-50 dark:bg-zinc-800 px-3 py-2 text-sm text-zinc-900 dark:text-zinc-100 placeholder-zinc-400 dark:placeholder-zinc-600 focus:border-brand-500 focus:outline-none" />
					</div>
					<div class="flex-1">
						<label for="password" class="mb-1.5 block text-xs text-zinc-400 dark:text-zinc-500">Password</label>
						<input id="password" name="password" type="password" required
							class="w-full rounded-md border border-zinc-300 dark:border-zinc-700 bg-zinc-50 dark:bg-zinc-800 px-3 py-2 text-sm text-zinc-900 dark:text-zinc-100 placeholder-zinc-400 dark:placeholder-zinc-600 focus:border-brand-500 focus:outline-none" />
					</div>
					<button type="submit"
						class="shrink-0 rounded-md border border-brand-500 bg-brand-500 dark:bg-brand-500/20 px-4 py-2 text-sm font-medium text-white dark:text-brand-300 hover:bg-brand-600 dark:hover:bg-brand-500/40 hover:text-white transition-colors">
						Create
					</button>
				</form>
			</div>
		{/if}

		{#if users.length === 0}
			<div class="rounded-xl border border-dashed border-zinc-300 dark:border-zinc-700 p-8 text-center">
				<p class="text-sm text-zinc-400 dark:text-zinc-500">No users yet.</p>
			</div>
		{:else}
			<div class="overflow-x-auto rounded-xl border border-zinc-200 dark:border-zinc-800">
				<table class="w-full text-sm">
					<thead>
						<tr class="border-b border-zinc-200 dark:border-zinc-800 bg-zinc-50/80 dark:bg-zinc-900/80">
							<th class="px-4 py-2.5 text-left text-[11px] font-semibold uppercase tracking-wider text-zinc-500 dark:text-zinc-400">Email</th>
							<th class="px-4 py-2.5 text-left text-[11px] font-semibold uppercase tracking-wider text-zinc-500 dark:text-zinc-400">Created</th>
							<th class="px-4 py-2.5"></th>
						</tr>
					</thead>
					<tbody class="divide-y divide-zinc-200/60 dark:divide-zinc-800/60">
						{#each users as user (user.ID)}
							{@const isSelf = user.ID === me?.user_id}
							<tr class="bg-white dark:bg-zinc-900 transition-colors hover:bg-zinc-50 dark:hover:bg-zinc-800/40">
								<td class="px-4 py-3">
									<span class="inline-flex items-center gap-2 text-zinc-800 dark:text-zinc-200">
										<svg class="h-3.5 w-3.5 shrink-0 text-zinc-300 dark:text-zinc-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
											<path
												stroke-linecap="round"
												stroke-linejoin="round"
												stroke-width="2"
												d="M21.75 6.75v10.5a2.25 2.25 0 0 1-2.25 2.25h-15a2.25 2.25 0 0 1-2.25-2.25V6.75m19.5 0A2.25 2.25 0 0 0 19.5 4.5h-15a2.25 2.25 0 0 0-2.25 2.25m19.5 0v.243a2.25 2.25 0 0 1-1.07 1.916l-7.5 4.615a2.25 2.25 0 0 1-2.36 0L3.32 8.91a2.25 2.25 0 0 1-1.07-1.916V6.75"
											/>
										</svg>
										{user.Email}
										{#if user.IsAdmin}
											<span class="rounded px-1.5 py-0.5 text-[10px] font-medium bg-brand-500/20 text-brand-500 dark:text-brand-300 border border-brand-500/30 dark:border-brand-600/40">admin</span>
										{/if}
									</span>
								</td>
								<td class="px-4 py-3 text-xs text-zinc-400 dark:text-zinc-500">{fmtDateOnly(user.CreatedAt)}</td>
								<td class="px-4 py-3 text-right whitespace-nowrap">
									{#if !isSelf}
										<button type="button" onclick={() => setAdmin(user)} class="mr-3 text-xs text-zinc-400 dark:text-zinc-600 transition-colors hover:text-zinc-700 dark:hover:text-zinc-300">
											{user.IsAdmin ? 'Remove admin' : 'Make admin'}
										</button>
										<button
											type="button"
											onclick={() => resetPasswordFor = resetPasswordFor === user.ID ? null : user.ID}
											class="text-xs text-zinc-400 dark:text-zinc-600 transition-colors hover:text-zinc-700 dark:hover:text-zinc-300"
										>
											Reset password
										</button>
										<button type="button" onclick={() => deleteUser(user)} class="ml-3 text-xs text-zinc-400 dark:text-zinc-600 transition-colors hover:text-red-400">Delete</button>
									{/if}
								</td>
							</tr>
							{#if resetPasswordFor === user.ID}
								<tr class="bg-zinc-50/60 dark:bg-zinc-800/30">
									<td colspan="3" class="px-4 py-3">
										{#if error && errorTab === 'users'}
											<div class="mb-2 rounded-md border border-red-500/20 bg-red-500/10 px-3 py-2 text-xs text-red-600 dark:text-red-400">{error}</div>
										{/if}
										<form onsubmit={(e) => resetPassword(e, user.ID)} class="flex items-center gap-2">
											<input
												name="newPassword"
												type="password"
												placeholder="New password"
												required
												minlength="8"
												class="flex-1 max-w-xs rounded-md border border-zinc-300 dark:border-zinc-700 bg-zinc-50 dark:bg-zinc-800 px-3 py-1.5 text-sm text-zinc-900 dark:text-zinc-100 placeholder-zinc-400 dark:placeholder-zinc-600 focus:border-brand-500 focus:outline-none"
											/>
											<button type="submit"
												class="rounded-md border border-brand-500 bg-brand-500 dark:bg-brand-500/20 px-3 py-1.5 text-xs font-medium text-white dark:text-brand-300 hover:bg-brand-600 dark:hover:bg-brand-500/40 hover:text-white transition-colors">
												Set password
											</button>
										</form>
									</td>
								</tr>
							{/if}
						{/each}
					</tbody>
				</table>
			</div>
		{/if}
	{/if}

	<!-- Groups tab -->
	{#if tab === 'groups'}
		<div class="mb-4 flex items-center justify-between">
			<p class="text-sm text-zinc-400 dark:text-zinc-500">{groups.length} {groups.length === 1 ? 'group' : 'groups'}</p>
			<button
				onclick={() => showGroupForm = !showGroupForm}
				class="rounded-md border border-brand-500 bg-brand-500 dark:bg-brand-500/20 px-3 py-1.5 text-xs font-medium text-white dark:text-brand-300 hover:bg-brand-600 dark:hover:bg-brand-500/40 hover:text-white transition-colors"
			>
				{showGroupForm ? 'Cancel' : 'New group'}
			</button>
		</div>

		{#if showGroupForm}
			<div class="mb-4 rounded-xl border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 p-5">
				<h2 class="mb-4 text-sm font-medium text-zinc-500 dark:text-zinc-400">New group</h2>
				{#if error && errorTab === 'groups'}
					<div class="mb-3 rounded-md border border-red-500/20 bg-red-500/10 px-3 py-2 text-xs text-red-600 dark:text-red-400">{error}</div>
				{/if}
				<form onsubmit={createGroup} class="flex gap-3">
					<input name="name" type="text" placeholder="Group name" required
						class="flex-1 rounded-md border border-zinc-300 dark:border-zinc-700 bg-zinc-50 dark:bg-zinc-800 px-3 py-2 text-sm text-zinc-900 dark:text-zinc-100 placeholder-zinc-400 dark:placeholder-zinc-600 focus:border-brand-500 focus:outline-none" />
					<button type="submit"
						class="rounded-md border border-brand-500 bg-brand-500 dark:bg-brand-500/20 px-4 py-2 text-sm font-medium text-white dark:text-brand-300 hover:bg-brand-600 dark:hover:bg-brand-500/40 hover:text-white transition-colors">
						Create
					</button>
				</form>
			</div>
		{/if}

		{#if groups.length === 0}
			<div class="rounded-xl border border-dashed border-zinc-300 dark:border-zinc-700 p-8 text-center">
				<p class="text-sm text-zinc-400 dark:text-zinc-500">No groups yet.</p>
			</div>
		{:else}
			<div class="space-y-2">
				{#each groups as group (group.ID)}
					<div class="flex items-center justify-between rounded-xl border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 px-4 py-3">
						<a href="/groups/{group.ID}" class="text-sm font-medium text-zinc-800 dark:text-zinc-200 hover:text-zinc-900 dark:hover:text-white transition-colors">
							{group.Name}
						</a>
						<button type="button" onclick={() => deleteGroup(group)} class="text-xs text-zinc-400 dark:text-zinc-600 transition-colors hover:text-red-400">Delete</button>
					</div>
				{/each}
			</div>
		{/if}
	{/if}
</div>
