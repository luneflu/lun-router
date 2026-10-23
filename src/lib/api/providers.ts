import type { ProviderModel } from '$lib/types';

export interface ModelPingResult {
	ok: boolean;
	model: string;
	latency_ms: number;
	error?: string;
	status?: number;
}

export interface BatchPingResult {
	ok: boolean;
	results: ModelPingResult[];
	total: number;
	passed: number;
	failed: number;
}

export interface FetchModelsResult {
	models: ProviderModel[];
	version: string;
}

export interface ChatGPTStatus {
	enabled: boolean;
	installed: boolean;
	has_cookies?: boolean;
	browser_path?: string;
	install_state?: string;
	install_progress?: number;
	install_error?: string;
	browser_running?: boolean;
	browser_pid?: number;
	browser_procs?: number;
	browser_rss_mb?: number;
	shell_enabled?: boolean;
	work_dir?: string;
	passthrough_tools?: boolean;
}

export async function fetchChatGPTStatus(): Promise<ChatGPTStatus> {
	const res = await fetch('/api/providers/chatgpt/status');
	if (!res.ok) {
		throw new Error(`Failed to fetch ChatGPT status: ${res.statusText}`);
	}
	return res.json();
}

export async function updateChatGPTStatus(enabled: boolean): Promise<{ ok: boolean; enabled: boolean }> {
	const res = await fetch('/api/providers/chatgpt/status', {
		method: 'PUT',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ enabled })
	});
	if (!res.ok) {
		const errData = await res.json().catch(() => ({}));
		throw new Error(errData.error || `Failed to update ChatGPT status: ${res.statusText}`);
	}
	return res.json();
}

export async function installChatGPTBrowser(): Promise<{ ok: boolean }> {
	const res = await fetch('/api/providers/chatgpt/install', {
		method: 'POST'
	});
	if (!res.ok) {
		const errData = await res.json().catch(() => ({}));
		throw new Error(errData.error || `Failed to start browser install: ${res.statusText}`);
	}
	return res.json();
}

export async function uploadChatGPTCookies(cookiesJson: string): Promise<{ ok: boolean; has_cookies: boolean }> {
	const res = await fetch('/api/providers/chatgpt/cookies', {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: cookiesJson
	});
	if (!res.ok) {
		const errData = await res.json().catch(() => ({}));
		throw new Error(errData.error || `Failed to upload cookies: ${res.statusText}`);
	}
	return res.json();
}

export async function deleteChatGPTCookies(): Promise<{ ok: boolean; has_cookies: boolean }> {
	const res = await fetch('/api/providers/chatgpt/cookies', {
		method: 'DELETE'
	});
	if (!res.ok) {
		const errData = await res.json().catch(() => ({}));
		throw new Error(errData.error || `Failed to clear cookies: ${res.statusText}`);
	}
	return res.json();
}

export async function updateChatGPTTools(payload: {
	shell_enabled?: boolean;
	work_dir?: string;
	passthrough_tools?: boolean;
}): Promise<{ ok: boolean; shell_enabled: boolean; work_dir: string; passthrough_tools: boolean }> {
	const res = await fetch('/api/providers/chatgpt/tools', {
		method: 'PUT',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(payload)
	});
	if (!res.ok) {
		const errData = await res.json().catch(() => ({}));
		throw new Error(errData.error || `Failed to update tool settings: ${res.statusText}`);
	}
	return res.json();
}

export async function fetchProviderModels(
	provider: string = 'opencode'
): Promise<FetchModelsResult> {
	const res = await fetch(`/api/providers/${provider}/models`);
	if (!res.ok) {
		throw new Error(`Failed to fetch models for provider ${provider}: ${res.statusText}`);
	}
	const data = await res.json();
	return {
		models: data.models || [],
		version: data.version || '1.19.0'
	};
}

export async function refreshOpenCodeVersion(): Promise<{
	ok: boolean;
	version: string;
	updated?: boolean;
	error?: string;
}> {
	const res = await fetch('/api/providers/opencode/version/refresh', { method: 'POST' });
	return res.json();
}

export async function refreshOpenCodeModels(): Promise<{
	ok: boolean;
	count: number;
	updated: boolean;
	error?: string;
}> {
	const res = await fetch('/api/providers/opencode/models/refresh', { method: 'POST' });
	return res.json();
}

export async function setModelEnabled(
	provider: string = 'opencode',
	model: string,
	enabled: boolean
): Promise<void> {
	const res = await fetch(`/api/providers/${provider}/models`, {
		method: 'PUT',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ model, enabled })
	});

	if (!res.ok) {
		const errData = await res.json().catch(() => ({}));
		throw new Error(errData.error || `Failed to update model status: ${res.statusText}`);
	}
}

export async function setBatchModelsEnabled(
	provider: string = 'opencode',
	models: string[],
	enabled: boolean
): Promise<void> {
	const res = await fetch(`/api/providers/${provider}/models`, {
		method: 'PUT',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ models, enabled })
	});

	if (!res.ok) {
		const errData = await res.json().catch(() => ({}));
		throw new Error(errData.error || `Failed to update models: ${res.statusText}`);
	}
}

// Ping model with empty prompt like 9router
export async function pingModel(model: string, prompt: string = ''): Promise<ModelPingResult> {
	const res = await fetch('/api/models/test', {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ model, prompt })
	});
	return res.json();
}

// Ping batch of models concurrently
export async function pingModelsBatch(
	models: string[],
	concurrency: number = 4,
	prompt: string = ''
): Promise<BatchPingResult> {
	const res = await fetch('/api/models/test', {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ models, concurrency, prompt })
	});
	return res.json();
}

// ---------------------------------------------------------------------------
// z.ai (GLM) provider
// ---------------------------------------------------------------------------

export interface ZaiStatus {
	provider?: string;
	enabled: boolean;
	installed?: boolean;
	browser_path?: string;
	install_state?: string;
	install_progress?: number;
	install_error?: string;
	browser_running?: boolean;
	browser_pid?: number;
	browser_procs?: number;
	browser_rss_mb?: number;
	passthrough_tools?: boolean;
	guest_mode?: boolean;
	error?: string;
}

export async function fetchZaiStatus(): Promise<ZaiStatus> {
	const res = await fetch('/api/providers/zai/status');
	if (!res.ok) {
		throw new Error(`Failed to fetch z.ai status: ${res.statusText}`);
	}
	return res.json();
}

export async function updateZaiStatus(enabled: boolean): Promise<{ enabled: boolean }> {
	const res = await fetch('/api/providers/zai/status', {
		method: 'PUT',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ enabled })
	});
	if (!res.ok) {
		const errData = await res.json().catch(() => ({}));
		throw new Error(errData.error || `Failed to update z.ai status: ${res.statusText}`);
	}
	return res.json();
}

export async function installZaiBrowser(): Promise<{ ok: boolean; state?: string }> {
	const res = await fetch('/api/providers/zai/install', { method: 'POST' });
	if (!res.ok) {
		const errData = await res.json().catch(() => ({}));
		throw new Error(errData.error || `Failed to start browser install: ${res.statusText}`);
	}
	return res.json();
}

export async function updateZaiTools(payload: {
	passthrough_tools?: boolean;
	browser_path?: string;
}): Promise<{ passthrough_tools: boolean }> {
	const res = await fetch('/api/providers/zai/tools', {
		method: 'PUT',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(payload)
	});
	if (!res.ok) {
		const errData = await res.json().catch(() => ({}));
		throw new Error(errData.error || `Failed to update z.ai tools: ${res.statusText}`);
	}
	return res.json();
}

// ---------------------------------------------------------------------------
// Custom Models Management
// ---------------------------------------------------------------------------

export interface CreateCustomModelPayload {
	id: string;
	context_length?: number;
	max_completion_tokens?: number;
	vision?: boolean;
	reasoning?: boolean;
	tools?: boolean;
	thinking_levels?: string[];
}

export async function addCustomModel(
	provider: string,
	payload: CreateCustomModelPayload
): Promise<{ success: boolean; model: ProviderModel }> {
	const res = await fetch(`/api/providers/${provider}/models/custom`, {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(payload)
	});
	if (!res.ok) {
		const errData = await res.json().catch(() => ({}));
		throw new Error(errData.error || `Failed to add custom model: ${res.statusText}`);
	}
	return res.json();
}

export async function deleteCustomModel(
	provider: string,
	modelId: string
): Promise<{ success: boolean }> {
	const cleanId = encodeURIComponent(modelId);
	const res = await fetch(`/api/providers/${provider}/models/custom?id=${cleanId}`, {
		method: 'DELETE'
	});
	if (!res.ok) {
		const errData = await res.json().catch(() => ({}));
		throw new Error(errData.error || `Failed to delete custom model: ${res.statusText}`);
	}
	return res.json();
}
