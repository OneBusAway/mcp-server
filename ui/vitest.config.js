import { defineConfig } from 'vitest/config';

// The Svelte plugin is intentionally omitted here: Phase U0 lands the test
// harness with no component tests yet, and @sveltejs/vite-plugin-svelte v7
// trips on hot-update inside Vitest's Vite server. Phase U4 wires the plugin
// back in with the correct hot: false shape once component tests exist.
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
