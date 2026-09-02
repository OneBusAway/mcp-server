import { defineConfig, devices } from '@playwright/test';

const port = Number(process.env.PORT ?? 4173);

export default defineConfig({
	testDir: 'tests/e2e',
	fullyParallel: true,
	forbidOnly: !!process.env.CI,
	retries: process.env.CI ? 2 : 0,
	workers: process.env.CI ? 1 : undefined,
	reporter: process.env.CI ? [['list'], ['html', { open: 'never' }]] : 'list',
	use: {
		baseURL: `http://127.0.0.1:${port}`,
		trace: 'on-first-retry',
	},
	projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }],
	webServer: {
		command: `PORT=${port} npm run preview -- --host 127.0.0.1 --port ${port}`,
		port,
		reuseExistingServer: !process.env.CI,
		timeout: 60_000,
	},
});
