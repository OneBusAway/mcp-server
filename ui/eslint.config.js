import js from '@eslint/js';
import svelte from 'eslint-plugin-svelte';
import globals from 'globals';
import prettier from 'eslint-config-prettier';
import svelteParser from 'svelte-eslint-parser';

export default [
	js.configs.recommended,
	...svelte.configs['flat/recommended'],
	prettier,
	...svelte.configs['flat/prettier'],
	{
		languageOptions: {
			ecmaVersion: 2024,
			sourceType: 'module',
			globals: {
				...globals.browser,
				...globals.node,
				$state: 'readonly',
				$derived: 'readonly',
				$effect: 'readonly',
				$props: 'readonly',
				$bindable: 'readonly',
				$inspect: 'readonly',
				$host: 'readonly',
			},
		},
	},
	{
		files: ['**/*.svelte'],
		languageOptions: {
			parser: svelteParser,
		},
	},
	{
		rules: {
			'no-unused-vars': ['error', { argsIgnorePattern: '^_', varsIgnorePattern: '^_' }],
			'no-console': ['warn', { allow: ['warn', 'error'] }],
			'no-empty': ['error', { allowEmptyCatch: true }],
		},
	},
	{
		ignores: [
			'.svelte-kit/',
			'build/',
			'node_modules/',
			'coverage/',
			'playwright-report/',
			'test-results/',
		],
	},
];
