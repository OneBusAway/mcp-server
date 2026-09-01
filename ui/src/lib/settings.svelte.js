function load(key, fallback) {
	try { return localStorage.getItem(key) ?? fallback; } catch { return fallback; }
}
function save(key, val) {
	try { localStorage.setItem(key, val); } catch {}
}

export const PROVIDERS = [
	{
		id: 'anthropic',
		label: 'Anthropic (Claude)',
		keyPlaceholder: 'sk-ant-…',
		keyHelp: 'Get yours at console.anthropic.com — separate from the Claude Pro subscription.',
		models: [
			{ id: 'claude-haiku-4-5-20251001', label: 'Claude Haiku 4.5 — Fast & cheap' },
			{ id: 'claude-sonnet-4-6',          label: 'Claude Sonnet 4.6 — Smarter' },
			{ id: 'claude-opus-4-7',            label: 'Claude Opus 4.7 — Most capable' },
		],
	},
	{
		id: 'groq',
		label: 'Groq (free tier)',
		keyPlaceholder: 'gsk_…',
		keyHelp: 'Free API at console.groq.com — no credit card required.',
		baseUrl: 'https://api.groq.com/openai/v1',
		models: [
			{ id: 'llama-3.3-70b-versatile', label: 'Llama 3.3 70B — Best quality' },
			{ id: 'openai/gpt-oss-20b',      label: 'GPT OSS 20B — Fastest (1000 t/s)' },
			{ id: 'openai/gpt-oss-120b',     label: 'GPT OSS 120B — Smartest' },
		],
	},
	{
		id: 'openrouter',
		label: 'OpenRouter',
		keyPlaceholder: 'sk-or-…',
		keyHelp: 'Free models at openrouter.ai — create account, no credit card required.',
		baseUrl: 'https://openrouter.ai/api/v1',
		models: [
			{ id: 'google/gemma-4-31b-it:free',             label: 'Gemma 4 31B — Google (free)' },
			{ id: 'nvidia/nemotron-3.5-lightning:free',      label: 'Nemotron 3.5 Lightning — NVIDIA (free)' },
			{ id: 'nvidia/nemotron-3-super-120b-a12b:free',  label: 'Nemotron 3 Super 120B — NVIDIA (free)' },
			{ id: 'minimax/minimax-m3:free',                 label: 'MiniMax M3 — 1M ctx (free)' },
			{ id: 'thinkingmachines/inkling:free',           label: 'Inkling — 1M ctx (free)' },
		],
	},
	{
		id: 'google-ai-studio',
		label: 'Google AI Studio',
		keyPlaceholder: 'AIza…',
		keyHelp: 'Free API at aistudio.google.com — create account, no credit card required.',
		baseUrl: 'https://generativelanguage.googleapis.com/v1beta/openai',
		models: [
			{ id: 'gemini-2.0-flash',   label: 'Gemini 2.0 Flash — Fast & free' },
			{ id: 'gemini-2.5-flash',   label: 'Gemini 2.5 Flash — Balanced' },
			{ id: 'gemini-2.5-pro',     label: 'Gemini 2.5 Pro — Most capable' },
		],
	},
	{
		id: 'openai',
		label: 'OpenAI',
		keyPlaceholder: 'sk-…',
		keyHelp: 'Get yours at platform.openai.com',
		baseUrl: 'https://api.openai.com/v1',
		models: [
			{ id: 'gpt-4o-mini', label: 'GPT-4o Mini — Cheap' },
			{ id: 'gpt-4o',     label: 'GPT-4o — Smarter' },
		],
	},
	{
		id: 'ollama',
		label: 'Ollama (local)',
		keyPlaceholder: 'ollama (no key needed)',
		keyHelp: 'Run models locally with ollama.ai — free, private, no API key.',
		baseUrl: 'http://localhost:11434/v1',
		models: [
			{ id: 'llama3.2',       label: 'Llama 3.2' },
			{ id: 'mistral',        label: 'Mistral' },
			{ id: 'qwen2.5-coder',  label: 'Qwen 2.5 Coder' },
		],
	},
	{
		id: 'llama-server',
		label: 'llama-server (local)',
		keyPlaceholder: 'llama-server (no key needed)',
		keyHelp: 'Run llama.cpp\'s llama-server locally — free, private, no API key. Start with: llama-server -hf Qwen/Qwen3-8B-GGUF:Q4_K_M --port 3000',
		baseUrl: 'http://localhost:3000/v1',
		models: [
			{ id: 'Qwen3-8B-Q4_K_M', label: 'Qwen3 8B (Q4_K_M) — active model' },
		],
	},
];

function loadJson(key, fallback) {
	try { return JSON.parse(localStorage.getItem(key) ?? 'null') ?? fallback; } catch { return fallback; }
}
function saveJson(key, val) {
	try { localStorage.setItem(key, JSON.stringify(val)); } catch {}
}

function createSettings() {
	let provider    = $state(load('provider', 'anthropic'));
	let apiKey      = $state(load('api_key', ''));
	let model       = $state(load('chat_model', 'claude-haiku-4-5-20251001'));
	let mapStyle    = $state(load('map_style', 'https://tiles.openfreemap.org/styles/bright'));
	let toolMode    = $state(load('tool_mode', 'rider'));

	return {
		get provider()   { return provider; },
		set provider(v)  { provider = v; save('provider', v); },

		get apiKey()     { return apiKey; },
		set apiKey(v)    { apiKey = v; save('api_key', v); },

		// Legacy alias used by existing chat code
		get anthropicKey() { return provider === 'anthropic' ? apiKey : ''; },

		get model()      { return model; },
		set model(v)     { model = v; save('chat_model', v); },

		get mapStyle()   { return mapStyle; },
		set mapStyle(v)  { mapStyle = v; save('map_style', v); },

		get toolMode()   { return toolMode; },
		set toolMode(v)  { toolMode = v; save('tool_mode', v); },

		get providerConfig() {
			return PROVIDERS.find((p) => p.id === provider) ?? PROVIDERS[0];
		},

		getCachedModels(providerId) {
			return loadJson(`fetched_models_${providerId}`, null);
		},
		setCachedModels(providerId, models) {
			saveJson(`fetched_models_${providerId}`, models);
		},
	};
}

export const settings = createSettings();
