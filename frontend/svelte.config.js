import adapter from '@sveltejs/adapter-static';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';

/** @type {import('@sveltejs/kit').Config} */
const config = {
	preprocess: vitePreprocess(),
	kit: {
		// SPA mode: the Go binary embeds build/ and serves index.html for
		// any path that is not a real file, so client-side routing works.
		adapter: adapter({ fallback: 'index.html' })
	}
};

export default config;
