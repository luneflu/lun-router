import type { KilocodeAccount, ProviderModel } from '$lib/types';

export interface DeviceCodeResponse {
	device_code: string;
	user_code: string;
	verification_uri: string;
	verification_uri_complete: string;
	expires_in: number;
	interval: number;
}

export interface PollResult {
	status: string; // 'approved' | 'pending' | 'access_denied' | 'expired_token' | 'poll_failed'
	success: boolean;
	account?: KilocodeAccount;
	error?: string;
}

export interface CreateAccountPayload {
	token: string;
	email?: string;
	orgId?: string;
}

export async function fetchKilocodeAccounts(): Promise<KilocodeAccount[]> {
	try {
		const res = await fetch('/api/providers/kilocode/accounts');
		if (res.ok) {
			const data = await res.json();
			return data.accounts || [];
		}
	} catch {
		// backend offline or restarting
	}
	return [];
}

export async function requestKilocodeDeviceCode(): Promise<DeviceCodeResponse> {
	const res = await fetch('/api/providers/kilocode/oauth/device-code', {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' }
	});

	if (!res.ok) {
		const err = await res.json().catch(() => ({}));
		throw new Error(err.error || `Failed to initiate device auth: ${res.statusText}`);
	}
	return res.json();
}

export async function pollKilocodeToken(deviceCode: string): Promise<PollResult> {
	const res = await fetch('/api/providers/kilocode/oauth/poll', {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ deviceCode })
	});

	if (!res.ok && res.status !== 400 && res.status !== 403 && res.status !== 410) {
		const err = await res.json().catch(() => ({}));
		throw new Error(err.error || `Poll request failed: ${res.statusText}`);
	}

	return res.json();
}

export async function createKilocodeAccount(payload: CreateAccountPayload): Promise<KilocodeAccount> {
	const res = await fetch('/api/providers/kilocode/accounts', {
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

export async function setKilocodeAccountActive(id: string, isActive: boolean): Promise<void> {
	const res = await fetch(`/api/providers/kilocode/accounts/${id}`, {
		method: 'PUT',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ isActive })
	});

	if (!res.ok) {
		const err = await res.json().catch(() => ({}));
		throw new Error(err.error || `Failed to update account: ${res.statusText}`);
	}
}

export async function deleteKilocodeAccount(id: string): Promise<void> {
	const res = await fetch(`/api/providers/kilocode/accounts/${id}`, {
		method: 'DELETE'
	});

	if (!res.ok) {
		const err = await res.json().catch(() => ({}));
		throw new Error(err.error || `Failed to delete account: ${res.statusText}`);
	}
}

export async function testKilocodeAccount(id: string): Promise<{ ok: boolean; latency_ms: number; error?: string }> {
	const res = await fetch(`/api/providers/kilocode/accounts/${id}/test`, {
		method: 'POST'
	});
	return res.json();
}

export async function fetchKilocodeModels(): Promise<ProviderModel[]> {
	try {
		const res = await fetch('/api/providers/kilocode/models');
		if (res.ok) {
			const data = await res.json();
			return data.models || [];
		}
	} catch {
		// backend offline
	}
	return [];
}

export async function refreshKilocodeModels(): Promise<{ ok: boolean; count: number; error?: string }> {
	const res = await fetch('/api/providers/kilocode/models/refresh', {
		method: 'POST'
	});
	return res.json();
}

export async function setKilocodeModelEnabled(model: string, enabled: boolean): Promise<void> {
	const res = await fetch('/api/providers/kilocode/models', {
		method: 'PUT',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ model, enabled })
	});

	if (!res.ok) {
		const err = await res.json().catch(() => ({}));
		throw new Error(err.error || `Failed to update model: ${res.statusText}`);
	}
}

export async function setKilocodeBatchModelsEnabled(models: string[], enabled: boolean): Promise<void> {
	const res = await fetch('/api/providers/kilocode/models', {
		method: 'PUT',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ models, enabled })
	});

	if (!res.ok) {
		const err = await res.json().catch(() => ({}));
		throw new Error(err.error || `Failed to update models: ${res.statusText}`);
	}
}
