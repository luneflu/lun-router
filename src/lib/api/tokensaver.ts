import type { TokenSaverStatus, TokenSaverTestResult } from '$lib/types';

export async function fetchTokenSaverStatus(): Promise<TokenSaverStatus> {
	const res = await fetch('/api/tokensaver');
	if (!res.ok) {
		throw new Error(`Failed to fetch Token Saver status: ${res.statusText}`);
	}
	return res.json();
}

export async function updateTokenSaver(rtkEnabled: boolean): Promise<TokenSaverStatus> {
	const res = await fetch('/api/tokensaver', {
		method: 'PUT',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ rtk_enabled: rtkEnabled })
	});
	if (!res.ok) {
		throw new Error(`Failed to update Token Saver: ${res.statusText}`);
	}
	return res.json();
}

export async function testTokenSaver(text: string): Promise<TokenSaverTestResult> {
	const res = await fetch('/api/tokensaver/test', {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ text })
	});
	if (!res.ok) {
		throw new Error(`Failed to test Token Saver: ${res.statusText}`);
	}
	return res.json();
}
