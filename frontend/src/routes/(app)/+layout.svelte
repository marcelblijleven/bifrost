<script lang="ts">
  import { page } from '$app/stores'
  import { goto } from '$app/navigation'
  import { onMount } from 'svelte'
  import type { Snippet } from 'svelte'
  import type { LayoutData } from './$types'
  import { theme } from '$lib/theme'
  import { api } from '$lib/api'

  let { data, children }: { data: LayoutData; children: Snippet } = $props()

  let menuOpen = $state(false)

  const allNavItems = [
    { href: '/', label: 'Dashboard', icon: 'M3 12l2-2m0 0l7-7 7 7M5 10v10a1 1 0 001 1h3m10-11l2 2m-2-2v10a1 1 0 01-1 1h-3m-6 0a1 1 0 001-1v-4a1 1 0 011-1h2a1 1 0 011 1v4a1 1 0 001 1m-6 0h6', adminOnly: false },
    { href: '/applications', label: 'Applications', icon: 'M3 7h18M3 12h18M3 17h18', adminOnly: false },
    { href: '/users', label: 'Team', icon: 'M18 18.72a9.094 9.094 0 0 0 3.741-.479 3 3 0 0 0-4.682-2.72m.94 3.198.001.031c0 .225-.012.447-.037.666A11.944 11.944 0 0 1 12 21c-2.17 0-4.207-.576-5.963-1.584A6.062 6.062 0 0 1 6 18.719m12 0a5.971 5.971 0 0 0-.941-3.197m0 0A5.995 5.995 0 0 0 12 12.75a5.995 5.995 0 0 0-5.058 2.772m0 0a3 3 0 0 0-4.681 2.72 8.986 8.986 0 0 0 3.74.477m.94-3.197a5.971 5.971 0 0 0-.94 3.197M15 6.75a3 3 0 1 1-6 0 3 3 0 0 1 6 0Zm6 3a2.25 2.25 0 1 1-4.5 0 2.25 2.25 0 0 1 4.5 0Zm-13.5 0a2.25 2.25 0 1 1-4.5 0 2.25 2.25 0 0 1 4.5 0Z', adminOnly: true },
    { href: '/docs', label: 'Docs', icon: 'M12 6.042A8.967 8.967 0 006 3.75c-1.052 0-2.062.18-3 .512v14.25A8.987 8.987 0 016 18c2.305 0 4.408.867 6 2.292m0-14.25a8.966 8.966 0 016-2.292c1.052 0 2.062.18 3 .512v14.25A8.987 8.987 0 0018 18a8.967 8.967 0 00-6 2.292m0-14.25v14.25', adminOnly: false },
  ]

  const navItems = $derived(allNavItems.filter(item => !item.adminOnly || data.user?.is_admin))

  function closeMenu() { menuOpen = false }

  async function signOut() {
    // Only the server can clear the httpOnly session cookie
    await api.auth.logout().catch(() => {})
    goto('/login')
  }

  onMount(() => theme.init())
</script>

<div class="flex h-full min-h-screen">
  <!-- Mobile header -->
  <header class="fixed inset-x-0 top-0 z-20 flex h-14 items-center justify-between border-b border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 px-4 lg:hidden">
    <span class="flex items-center gap-2 text-base font-bold tracking-tight text-zinc-900 dark:text-zinc-100"><img src="/logo.svg" alt="Bifrost" class="h-7 w-7" /> Bifrost</span>
    <button
      type="button"
      onclick={() => (menuOpen = !menuOpen)}
      class="rounded-md p-2 text-zinc-500 dark:text-zinc-400 hover:bg-zinc-100 dark:hover:bg-zinc-800 hover:text-zinc-900 dark:hover:text-zinc-100"
      aria-label="Toggle menu"
    >
      {#if menuOpen}
        <svg class="h-5 w-5" fill="none" stroke="currentColor" stroke-width="1.5" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
        </svg>
      {:else}
        <svg class="h-5 w-5" fill="none" stroke="currentColor" stroke-width="1.5" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" d="M3.75 6.75h16.5M3.75 12h16.5m-16.5 5.25h16.5" />
        </svg>
      {/if}
    </button>
  </header>

  <!-- Mobile menu overlay -->
  {#if menuOpen}
    <div
      class="fixed inset-0 z-10 bg-black/50 lg:hidden"
      role="button"
      tabindex="-1"
      onclick={closeMenu}
      onkeydown={closeMenu}
    ></div>
  {/if}

  <!-- Sidebar -->
  <aside class="
    fixed inset-y-0 left-0 z-30 flex w-60 flex-col border-r border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900
    transition-transform duration-200
    lg:static lg:translate-x-0
    {menuOpen ? 'translate-x-0' : '-translate-x-full'}
  ">
    <div class="flex h-14 items-center border-b border-zinc-200 dark:border-zinc-800 px-5">
      <span class="flex items-center gap-2 text-base font-bold tracking-tight text-zinc-900 dark:text-zinc-100"><img src="/logo.svg" alt="Bifrost" class="h-7 w-7" /> Bifrost</span>
    </div>

    <nav class="flex-1 space-y-0.5 p-3">
      {#each navItems as item}
        {@const active = item.href === '/' ? $page.url.pathname === '/' : $page.url.pathname.startsWith(item.href)}
        <a
          href={item.href}
          onclick={closeMenu}
          class="flex items-center gap-3 rounded-lg px-3 py-2 text-sm font-medium transition {active
            ? 'bg-zinc-100 dark:bg-zinc-800 text-zinc-900 dark:text-zinc-100'
            : 'text-zinc-500 dark:text-zinc-400 hover:bg-zinc-100 dark:hover:bg-zinc-800/60 hover:text-zinc-900 dark:hover:text-zinc-100'}"
        >
          <svg class="h-4 w-4 shrink-0" fill="none" stroke="currentColor" stroke-width="1.5" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" d={item.icon} />
          </svg>
          {item.label}
        </a>
      {/each}
    </nav>

    <div class="border-t border-zinc-200 dark:border-zinc-800 p-3">
      <a
        href="/account"
        onclick={closeMenu}
        class="mb-2 block rounded-lg px-3 py-1 transition hover:bg-zinc-100 dark:hover:bg-zinc-800"
      >
        <p class="truncate text-xs text-zinc-400 dark:text-zinc-500">{data.user?.email}</p>
        {#if data.user?.is_admin}
          <span class="mt-1 inline-block rounded px-2 py-0.5 text-xs font-medium bg-brand-500/20 text-brand-500 dark:text-brand-300 border border-brand-500/30 dark:border-brand-600/40">admin</span>
        {/if}
      </a>

      <button
        type="button"
        onclick={() => theme.toggle()}
        class="mb-1 flex w-full items-center gap-3 rounded-lg px-3 py-2 text-sm font-medium text-zinc-500 dark:text-zinc-400 transition hover:bg-zinc-100 dark:hover:bg-zinc-800 hover:text-zinc-700 dark:hover:text-zinc-100"
        aria-label="Toggle theme"
      >
        {#if $theme === 'dark'}
          <svg class="h-4 w-4 shrink-0" fill="none" stroke="currentColor" stroke-width="1.5" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" d="M12 3v2.25m6.364.386-1.591 1.591M21 12h-2.25m-.386 6.364-1.591-1.591M12 18.75V21m-4.773-4.227-1.591 1.591M5.25 12H3m4.227-4.773L5.636 5.636M15.75 12a3.75 3.75 0 1 1-7.5 0 3.75 3.75 0 0 1 7.5 0Z" />
          </svg>
          Light mode
        {:else}
          <svg class="h-4 w-4 shrink-0" fill="none" stroke="currentColor" stroke-width="1.5" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" d="M21.752 15.002A9.72 9.72 0 0 1 18 15.75c-5.385 0-9.75-4.365-9.75-9.75 0-1.33.266-2.597.748-3.752A9.753 9.753 0 0 0 3 11.25C3 16.635 7.365 21 12.75 21a9.753 9.753 0 0 0 9.002-5.998Z" />
          </svg>
          Dark mode
        {/if}
      </button>

      <button
        type="button"
        onclick={signOut}
        class="flex w-full items-center gap-3 rounded-lg px-3 py-2 text-sm font-medium text-zinc-500 dark:text-zinc-400 transition hover:bg-red-50 dark:hover:bg-red-950/40 hover:text-red-500 dark:hover:text-red-400"
      >
        <svg class="h-4 w-4 shrink-0" fill="none" stroke="currentColor" stroke-width="1.5" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" d="M15.75 9V5.25A2.25 2.25 0 0 0 13.5 3h-6a2.25 2.25 0 0 0-2.25 2.25v13.5A2.25 2.25 0 0 0 7.5 21h6a2.25 2.25 0 0 0 2.25-2.25V15M12 9l-3 3m0 0 3 3m-3-3h12.75" />
        </svg>
        Sign out
      </button>
    </div>
  </aside>

  <!-- Main content -->
  <main class="flex-1 overflow-auto pt-14 lg:pt-0">
    {@render children()}
  </main>
</div>
