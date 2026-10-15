import type { AntigravityAccount, ProviderModel, AntigravityAccountUsage } from '$lib/types';

export interface CreateAccountPayload {
	email?: string;
	projectId?: string;
	accessToken?: string;
	refreshToken?: string;
	code?: string;
	redirectUri?: string;
}

export function getAntigravityClientId(): string {
	return [
		'1071006',
		'060591',
		'-tmhssin2h21lcre',
		'235vtolojh4g403ep',
		'.apps.',
		'googleusercontent.com'
	].join('');
}

export const ANTIGRAVITY_SCOPES = [
	'https://www.googleapis.com/auth/cloud-platform',
	'https://www.googleapis.com/auth/userinfo.email',
	'https://www.googleapis.com/auth/userinfo.profile',
	'https://www.googleapis.com/auth/cclog',
	'https://www.googleapis.com/auth/experimentsandconfigs'
];

export function buildClientOAuthUrl(redirectUri?: string): string {
	const origin = typeof window !== 'undefined' ? window.location.origin : 'http://localhost:20128';
	const uri = redirectUri || `${origin}/callback`;
	const state = Math.random().toString(36).substring(2);
	const params = new URLSearchParams({
		client_id: getAntigravityClientId(),
		response_type: 'code',
		redirect_uri: uri,
		scope: ANTIGRAVITY_SCOPES.join(' '),
		state,
		access_type: 'offline',
		prompt: 'consent'
	});
	return `https://accounts.google.com/o/oauth2/v2/auth?${params.toString()}`;
}

export async function fetchAntigravityAccounts(): Promise<AntigravityAccount[]> {
	try {
		const res = await fetch('/api/providers/antigravity/accounts');
		if (res.ok) {
			const data = await res.json();
			return data.accounts || [];
		}
	} catch {
		// backend offline or restarting
	}
	return [];
}

export async function createAntigravityAccount(
	payload: CreateAccountPayload
): Promise<AntigravityAccount> {
	const res = await fetch('/api/providers/antigravity/accounts', {
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

export async function setAntigravityAccountActive(id: string, isActive: boolean): Promise<void> {
	const res = await fetch(`/api/providers/antigravity/accounts/${id}`, {
		method: 'PUT',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ isActive })
	});

	if (!res.ok) {
		const err = await res.json().catch(() => ({}));
		throw new Error(err.error || `Failed to update account: ${res.statusText}`);
	}
}

export async function deleteAntigravityAccount(id: string): Promise<void> {
	const res = await fetch(`/api/providers/antigravity/accounts/${id}`, {
		method: 'DELETE'
	});

	if (!res.ok) {
		const err = await res.json().catch(() => ({}));
		throw new Error(err.error || `Failed to delete account: ${res.statusText}`);
	}
}

export async function refreshAntigravityAccount(
	id: string
): Promise<{ ok: boolean; expiresAt?: string; error?: string }> {
	const res = await fetch(`/api/providers/antigravity/accounts/${id}/refresh`, {
		method: 'POST'
	});
	return res.json();
}

export async function getAntigravityOAuthUrl(
	redirectUri?: string
): Promise<{ authUrl: string; clientId: string }> {
	try {
		const q = redirectUri ? `?redirect_uri=${encodeURIComponent(redirectUri)}` : '';
		const res = await fetch(`/api/providers/antigravity/oauth/auth-url${q}`);
		if (res.ok) {
			return await res.json();
		}
	} catch {
		// Fallback to client generation
	}
	return {
		authUrl: buildClientOAuthUrl(redirectUri),
		clientId: getAntigravityClientId()
	};
}

export async function fetchAntigravityModels(): Promise<{
	models: ProviderModel[];
	version: string;
}> {
	try {
		const res = await fetch('/api/providers/antigravity/models');
		if (res.ok) {
			const data = await res.json();
			return {
				models: data.models || [],
				version: data.version || '2.11.0'
			};
		}
	} catch {
		// Fallback
	}

	return {
		models: [
			'gemini-3.8-flash-high',
			'gemini-3.8-flash-medium',
			'gemini-3.8-flash-low',
			'gemini-3.8-flash',
			'gemini-3.7-flash-high',
			'gemini-3.7-flash-medium',
			'gemini-3.7-flash-low',
			'gemini-3.6-flash-high',
			'gemini-3.6-flash-medium',
			'gemini-3.6-flash-low',
			'gemini-3.1-pro-high',
			'gemini-3.1-pro-low',
			'claude-sonnet-5-5-high',
			'claude-sonnet-5-5-medium',
			'claude-sonnet-5-5-low',
			'claude-sonnet-5-5',
			'claude-opus-5-5-high',
			'claude-opus-5-5-medium',
			'claude-opus-5-5-low',
			'claude-opus-5-5',
			'claude-sonnet-4-6',
			'claude-opus-4-6-thinking',
			'gpt-oss-120b-medium',
			'gemini-3.1-flash-image'
		].map((id) => ({ id, enabled: true })),
		version: '2.11.0'
	};
}

export async function setAntigravityModelEnabled(model: string, enabled: boolean): Promise<void> {
	const res = await fetch('/api/providers/antigravity/models', {
		method: 'PUT',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ model, enabled })
	});

	if (!res.ok) {
		const err = await res.json().catch(() => ({}));
		throw new Error(err.error || `Failed to update model status: ${res.statusText}`);
	}
}

export async function setAntigravityBatchModelsEnabled(
	models: string[],
	enabled: boolean
): Promise<void> {
	const res = await fetch('/api/providers/antigravity/models', {
		method: 'PUT',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ models, enabled })
	});

	if (!res.ok) {
		const err = await res.json().catch(() => ({}));
		throw new Error(err.error || `Failed to update models: ${res.statusText}`);
	}
}

export async function fetchAntigravityQuota(
	accountId?: string,
	force = false
): Promise<Record<string, AntigravityAccountUsage>> {
	const url = accountId
		? `/api/providers/antigravity/accounts/${accountId}/quota${force ? '?force=true' : ''}`
		: `/api/providers/antigravity/quota${force ? '?force=true' : ''}`;

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
