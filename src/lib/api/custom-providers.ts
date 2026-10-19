import type {
	CustomProvider,
	CreateCustomProviderPayload,
	UpdateCustomProviderPayload,
	ProviderModel
} from '$lib/types';
import type { CreateCustomModelPayload } from './providers';

export async function fetchCustomProviders(): Promise<CustomProvider[]> {
	const res = await fetch('/api/providers/custom');
	if (!res.ok) {
		throw new Error(`Failed to fetch custom providers: ${res.statusText}`);
	}
	const data = await res.json();
	return data.providers || [];
}

export async function createCustomProvider(
	payload: CreateCustomProviderPayload
): Promise<CustomProvider> {
	const res = await fetch('/api/providers/custom', {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(payload)
	});
	if (!res.ok) {
		const errData = await res.json().catch(() => ({}));
		throw new Error(errData.error || `Failed to create provider: ${res.statusText}`);
	}
	const data = await res.json();
	return data.provider;
}

export async function updateCustomProvider(
	id: string,
	payload: UpdateCustomProviderPayload
): Promise<CustomProvider> {
	const res = await fetch(`/api/providers/custom/${encodeURIComponent(id)}`, {
		method: 'PUT',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(payload)
	});
	if (!res.ok) {
		const errData = await res.json().catch(() => ({}));
		throw new Error(errData.error || `Failed to update provider: ${res.statusText}`);
	}
	const data = await res.json();
	return data.provider;
}

export async function deleteCustomProvider(id: string): Promise<void> {
	const res = await fetch(`/api/providers/custom/${encodeURIComponent(id)}`, {
		method: 'DELETE'
	});
	if (!res.ok) {
		const errData = await res.json().catch(() => ({}));
		throw new Error(errData.error || `Failed to delete provider: ${res.statusText}`);
	}
}

export async function syncCustomProviderModels(id: string): Promise<{
	ok: boolean;
	count: number;
	models: string[];
}> {
	const res = await fetch(`/api/providers/custom/${encodeURIComponent(id)}/sync`, {
		method: 'POST'
	});
	if (!res.ok) {
		const errData = await res.json().catch(() => ({}));
		throw new Error(errData.error || `Failed to sync models: ${res.statusText}`);
	}
	return res.json();
}

export async function fetchCustomProviderModels(id: string): Promise<ProviderModel[]> {
	const res = await fetch(`/api/providers/custom/${encodeURIComponent(id)}/models`);
	if (!res.ok) {
		throw new Error(`Failed to fetch models: ${res.statusText}`);
	}
	const data = await res.json();
	return data.models || [];
}

export async function updateCustomProviderModels(
	id: string,
	payload: { model?: string; models?: string[]; enabled: boolean }
): Promise<void> {
	const res = await fetch(`/api/providers/custom/${encodeURIComponent(id)}/models`, {
		method: 'PUT',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(payload)
	});
	if (!res.ok) {
		const errData = await res.json().catch(() => ({}));
		throw new Error(errData.error || `Failed to update models: ${res.statusText}`);
	}
}

export async function addCustomProviderModel(
	id: string,
	payload: CreateCustomModelPayload
): Promise<ProviderModel> {
	const res = await fetch(`/api/providers/custom/${encodeURIComponent(id)}/models/custom`, {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(payload)
	});
	if (!res.ok) {
		const errData = await res.json().catch(() => ({}));
		throw new Error(errData.error || `Failed to add model: ${res.statusText}`);
	}
	const data = await res.json();
	return data.model;
}

export async function deleteCustomProviderModel(id: string, modelId: string): Promise<void> {
	const res = await fetch(
		`/api/providers/custom/${encodeURIComponent(id)}/models/custom?id=${encodeURIComponent(modelId)}`,
		{
			method: 'DELETE'
		}
	);
	if (!res.ok) {
		const errData = await res.json().catch(() => ({}));
		throw new Error(errData.error || `Failed to delete model: ${res.statusText}`);
	}
}
