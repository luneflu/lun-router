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
