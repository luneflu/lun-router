import type { CompactorStatus, CompactorTestResult } from '$lib/types';

export async function fetchCompactorStatus(): Promise<CompactorStatus> {
	const res = await fetch('/api/compactor');
	if (!res.ok) {
		throw new Error(`Failed to fetch compactor status: ${res.statusText}`);
	}
	return res.json();
}

export async function updateCompactor(enabled: boolean): Promise<CompactorStatus> {
	const res = await fetch('/api/compactor', {
		method: 'PUT',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ enabled })
	});
	if (!res.ok) {
		throw new Error(`Failed to update compactor config: ${res.statusText}`);
	}
	return res.json();
}

export async function testCompactorCompress(text: string): Promise<CompactorTestResult> {
	const res = await fetch('/api/compactor/test', {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ text })
	});
	if (!res.ok) {
		throw new Error(`Failed to test compactor compression: ${res.statusText}`);
	}
	return res.json();
}
