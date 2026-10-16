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
