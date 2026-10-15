import type { PonytailStatus, PonytailTestResult } from '$lib/types';

export async function fetchPonytailStatus(): Promise<PonytailStatus> {
	const res = await fetch('/api/ponytail');
	if (!res.ok) {
		throw new Error(`Failed to fetch Ponytail status: ${res.statusText}`);
	}
	return res.json();
}

export async function updatePonytailConfig(payload: {
	enabled?: boolean;
	mode?: string;
	auto_update?: boolean;
	custom_prompt?: string;
}): Promise<PonytailStatus> {
	const res = await fetch('/api/ponytail', {
		method: 'PUT',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(payload)
	});
	if (!res.ok) {
		throw new Error(`Failed to update Ponytail config: ${res.statusText}`);
	}
	return res.json();
}

export async function syncPonytailUpstream(): Promise<{
	ok: boolean;
	updated: boolean;
	error?: string;
	status: PonytailStatus;
}> {
	const res = await fetch('/api/ponytail/sync', {
		method: 'POST'
	});
	if (!res.ok) {
		const data = await res.json().catch(() => null);
		throw new Error(data?.error || `Failed to sync with upstream Ponytail: ${res.statusText}`);
	}
	return res.json();
}

export async function testPonytailPrompt(payload: {
	provider?: string;
	model?: string;
	mode?: string;
	user_prompt?: string;
	system_prior?: string;
}): Promise<PonytailTestResult> {
	const res = await fetch('/api/ponytail/test', {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(payload)
	});
	if (!res.ok) {
		throw new Error(`Failed to test Ponytail prompt: ${res.statusText}`);
	}
	return res.json();
}
