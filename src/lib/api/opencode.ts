import type { OpenCodeAccount, OpenCodeAccountUsage } from '$lib/types';

export interface OpenCodeDeviceAuthorization {
	ok: boolean;
	server: string;
	device_code: string;
	user_code: string;
	verification_uri: string;
	verification_uri_complete: string;
	expires_in: number;
	interval: number;
}

export interface OpenCodeOAuthPollResult {
	ok: boolean;
	status: 'pending' | 'approved' | 'error';
	account?: OpenCodeAccount;
	error?: string;
}

export async function fetchOpenCodeAccounts(): Promise<OpenCodeAccount[]> {
	try {
		const res = await fetch('/api/providers/opencode/accounts');
		if (res.ok) {
			const data = await res.json();
			return data.accounts || [];
		}
	} catch {
		// backend offline or restarting
	}
	return [];
}

export async function startOpenCodeOAuth(): Promise<OpenCodeDeviceAuthorization> {
	const res = await fetch('/api/providers/opencode/accounts/oauth/start', {
		method: 'POST'
	});
	const data = await res.json().catch(() => ({}));
	if (!res.ok || !data.ok) {
		throw new Error(data.error || `Failed to start OpenCode login: ${res.statusText}`);
	}
	return data;
}

export async function pollOpenCodeOAuth(deviceCode: string): Promise<OpenCodeOAuthPollResult> {
	const res = await fetch('/api/providers/opencode/accounts/oauth/poll', {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ device_code: deviceCode })
	});
	return res.json();
}

export async function setOpenCodeAccountActive(id: string, isActive: boolean): Promise<void> {
	const res = await fetch(`/api/providers/opencode/accounts/${id}`, {
		method: 'PUT',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ isActive })
	});

	if (!res.ok) {
		const err = await res.json().catch(() => ({}));
		throw new Error(err.error || `Failed to update account: ${res.statusText}`);
	}
}

export async function deleteOpenCodeAccount(id: string): Promise<void> {
	const res = await fetch(`/api/providers/opencode/accounts/${id}`, {
		method: 'DELETE'
	});

	if (!res.ok) {
		const err = await res.json().catch(() => ({}));
		throw new Error(err.error || `Failed to delete account: ${res.statusText}`);
	}
}

export async function testOpenCodeAccount(
	id: string
): Promise<{ ok: boolean; latency_ms: number; error?: string }> {
	const res = await fetch(`/api/providers/opencode/accounts/${id}/test`, {
		method: 'POST'
	});
	return res.json();
}

export async function fetchOpenCodeQuota(
	accountId?: string,
	force = false
): Promise<Record<string, OpenCodeAccountUsage>> {
	const url = accountId
		? `/api/providers/opencode/accounts/${accountId}/quota${force ? '?force=true' : ''}`
		: `/api/providers/opencode/quota${force ? '?force=true' : ''}`;

	const res = await fetch(url);
	if (!res.ok) {
		const err = await res.json().catch(() => ({}));
		throw new Error(err.error || `Failed to fetch quota: ${res.statusText}`);
	}

	const data = await res.json();
	if (accountId) {
		return { [accountId]: data };
	}
	return data.usages || {};
}
