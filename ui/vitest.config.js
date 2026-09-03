import { defineConfig } from 'vitest/config';

// The Svelte plugin is intentionally omitted: @sveltejs/vite-plugin-svelte v7
// trips on hot-update inside Vitest's Vite server. Re-add it with hot: false
// once component tests are introduced.
export default defineConfig({
	test: {
		environment: 'jsdom',
		globals: true,
		include: ['src/**/*.test.js', 'tests/unit/**/*.test.js'],
		exclude: ['tests/e2e/**', 'node_modules/**', '.svelte-kit/**', 'build/**'],
		passWithNoTests: true,
		coverage: {
			provider: 'v8',
			reporter: ['text', 'lcov'],
			include: ['src/lib/**'],
			exclude: ['src/lib/components/**/*.svelte', 'src/**/*.test.*'],
			thresholds: {
				lines: 70,
				functions: 70,
				statements: 70,
				branches: 60,
			},
		},
	},
});
