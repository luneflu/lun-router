import type { CommandCodeAccount, ProviderModel } from '$lib/types';

export interface CreateAccountPayload {
	apiKey: string;
	name?: string;
}

export async function fetchCommandCodeAccounts(): Promise<CommandCodeAccount[]> {
	try {
		const res = await fetch('/api/providers/commandcode/accounts');
		if (res.ok) {
			const data = await res.json();
			return data.accounts || [];
		}
	} catch {
		// backend offline or restarting
	}
	return [];
}

export async function createCommandCodeAccount(
	payload: CreateAccountPayload
): Promise<CommandCodeAccount> {
	const res = await fetch('/api/providers/commandcode/accounts', {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(payload)
	});

	if (!res.ok) {
		const err = await res.json().catch(() => ({}));
		throw new Error(err.error || `Failed to create account: ${res.statusText}`);
	}
	return res.json();
}

export async function setCommandCodeAccountActive(id: string, isActive: boolean): Promise<void> {
	const res = await fetch(`/api/providers/commandcode/accounts/${id}`, {
		method: 'PUT',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ isActive })
	});

	if (!res.ok) {
		const err = await res.json().catch(() => ({}));
		throw new Error(err.error || `Failed to update account: ${res.statusText}`);
	}
}

export async function deleteCommandCodeAccount(id: string): Promise<void> {
	const res = await fetch(`/api/providers/commandcode/accounts/${id}`, {
		method: 'DELETE'
	});

	if (!res.ok) {
		const err = await res.json().catch(() => ({}));
		throw new Error(err.error || `Failed to delete account: ${res.statusText}`);
	}
}

export async function testCommandCodeAccount(
	id: string
): Promise<{ ok: boolean; latency_ms: number; error?: string }> {
	const res = await fetch(`/api/providers/commandcode/accounts/${id}/test`, {
		method: 'POST'
	});
	return res.json();
}

export async function fetchCommandCodeModels(): Promise<ProviderModel[]> {
	try {
		const res = await fetch('/api/providers/commandcode/models');
		if (res.ok) {
			const data = await res.json();
			return data.models || [];
		}
	} catch {
		// backend offline
	}
	return [];
}

export async function refreshCommandCodeModels(): Promise<{
	ok: boolean;
	count: number;
	error?: string;
}> {
	const res = await fetch('/api/providers/commandcode/models/refresh', {
		method: 'POST'
	});
	return res.json();
}

export async function setCommandCodeModelEnabled(model: string, enabled: boolean): Promise<void> {
	const res = await fetch('/api/providers/commandcode/models', {
		method: 'PUT',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ model, enabled })
	});

	if (!res.ok) {
		const err = await res.json().catch(() => ({}));
		throw new Error(err.error || `Failed to update model: ${res.statusText}`);
	}
}

export async function setCommandCodeBatchModelsEnabled(
	models: string[],
	enabled: boolean
): Promise<void> {
	const res = await fetch('/api/providers/commandcode/models', {
		method: 'PUT',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ models, enabled })
	});

	if (!res.ok) {
		const err = await res.json().catch(() => ({}));
		throw new Error(err.error || `Failed to update models: ${res.statusText}`);
	}
}
