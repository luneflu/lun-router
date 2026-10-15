import type { RTKStatus, RTKTestResult } from '$lib/types';

export async function fetchRTKStatus(): Promise<RTKStatus> {
	const res = await fetch('/api/rtk');
	if (!res.ok) {
		throw new Error(`Failed to fetch RTK status: ${res.statusText}`);
	}
	return res.json();
}

export async function updateRTKConfig(rtkEnabled: boolean): Promise<RTKStatus> {
	const res = await fetch('/api/rtk', {
		method: 'PUT',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ rtk_enabled: rtkEnabled })
	});
	if (!res.ok) {
		throw new Error(`Failed to update RTK config: ${res.statusText}`);
	}
	return res.json();
}

export async function testRTKCompress(text: string): Promise<RTKTestResult> {
	const res = await fetch('/api/rtk/test', {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ text })
	});
	if (!res.ok) {
		throw new Error(`Failed to test RTK compression: ${res.statusText}`);
	}
	return res.json();
}

// Aliases for backward compatibility
export const fetchTokenSaverStatus = fetchRTKStatus;
export const updateTokenSaver = updateRTKConfig;
export const testTokenSaver = testRTKCompress;
