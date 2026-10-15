import type { CavemanStatus, CavemanTestResult } from '$lib/types';

export async function fetchCavemanStatus(): Promise<CavemanStatus> {
	const res = await fetch('/api/caveman');
	if (!res.ok) {
		throw new Error(`Failed to fetch Caveman status: ${res.statusText}`);
	}
	return res.json();
}

export async function updateCavemanConfig(payload: {
	enabled?: boolean;
	mode?: string;
	auto_update?: boolean;
	custom_prompt?: string;
}): Promise<CavemanStatus> {
	const res = await fetch('/api/caveman', {
		method: 'PUT',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(payload)
	});
	if (!res.ok) {
		throw new Error(`Failed to update Caveman config: ${res.statusText}`);
	}
	return res.json();
}

export async function syncCavemanUpstream(): Promise<{
	ok: boolean;
	updated: boolean;
	error?: string;
	status: CavemanStatus;
}> {
	const res = await fetch('/api/caveman/sync', {
		method: 'POST'
	});
	if (!res.ok) {
		const data = await res.json().catch(() => null);
		throw new Error(data?.error || `Failed to sync with upstream Caveman: ${res.statusText}`);
	}
	return res.json();
}

export async function testCavemanPrompt(payload: {
	provider?: string;
	model?: string;
	mode?: string;
	user_prompt?: string;
	system_prior?: string;
}): Promise<CavemanTestResult> {
	const res = await fetch('/api/caveman/test', {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(payload)
	});
	if (!res.ok) {
		throw new Error(`Failed to test Caveman prompt: ${res.statusText}`);
	}
	return res.json();
}
