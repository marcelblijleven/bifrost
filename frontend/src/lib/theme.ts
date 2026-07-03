import { writable } from 'svelte/store';
import { browser } from '$app/environment';

function createTheme() {
	const { subscribe, set } = writable<'light' | 'dark'>('light');

	return {
		subscribe,
		init() {
			if (!browser) return;
			const stored = localStorage.getItem('theme');
			const prefersDark = window.matchMedia('(prefers-color-scheme: dark)').matches;
			const initial =
				stored === 'dark' || stored === 'light'
					? (stored as 'light' | 'dark')
					: prefersDark
						? 'dark'
						: 'light';
			set(initial);
			document.documentElement.classList.toggle('dark', initial === 'dark');
		},
		toggle() {
			if (!browser) return;
			const isDark = document.documentElement.classList.toggle('dark');
			const next = isDark ? 'dark' : 'light';
			localStorage.setItem('theme', next);
			set(next);
		}
	};
}

export const theme = createTheme();
