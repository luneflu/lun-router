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
