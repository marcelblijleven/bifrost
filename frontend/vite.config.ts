import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';

export default defineConfig({
	plugins: [sveltekit()],
	server: {
		// In dev the Go backend runs separately on :8080; proxying keeps the
		// browser same-origin so the httpOnly session cookie works there too.
		proxy: {
			'/api': 'http://localhost:8080'
		}
	}
});
