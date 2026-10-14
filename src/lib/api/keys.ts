import type { APIKey, CreateKeyPayload } from '$lib/types';

export async function fetchApiKeys(): Promise<APIKey[]> {
	const res = await fetch('/api/keys');
	if (!res.ok) {
		throw new Error(`Failed to fetch keys (${res.status} ${res.statusText})`);
	}
	const data = await res.json();
	return data.keys || [];
}

export async function createApiKey(payload: CreateKeyPayload): Promise<APIKey> {
	const res = await fetch('/api/keys', {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(payload)
	});

	if (!res.ok) {
		const errData = await res.json().catch(() => ({}));
		throw new Error(errData.error || 'Failed to create key');
	}

	return res.json();
}

export async function updateApiKeyActive(id: string, isActive: boolean): Promise<void> {
	const res = await fetch(`/api/keys/${id}`, {
		method: 'PUT',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ isActive })
	});
	if (!res.ok) {
		throw new Error(`Failed to update key status: ${res.statusText}`);
	}
}

export async function deleteApiKey(id: string): Promise<void> {
	const res = await fetch(`/api/keys/${id}`, { method: 'DELETE' });
	if (!res.ok) {
		throw new Error(`Failed to delete key: ${res.statusText}`);
	}
}
