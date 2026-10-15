import type { AuthStatus } from '$lib/types';

export async function fetchAuthStatus(): Promise<AuthStatus> {
	const res = await fetch('/api/auth/status');
	if (!res.ok) {
		throw new Error(`Failed to fetch auth status: ${res.statusText}`);
	}
	return res.json();
}

export async function setupAuth(
	password: string
): Promise<{ success: boolean; token?: string; error?: string }> {
	const res = await fetch('/api/auth/setup', {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ password })
	});
	const data = await res.json();
	if (!res.ok) {
		throw new Error(data.error || 'Failed to setup password');
	}
	return data;
}

export async function loginAuth(
	password: string
): Promise<{ success: boolean; token?: string; error?: string }> {
	const res = await fetch('/api/auth/login', {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ password })
	});
	const data = await res.json();
	if (!res.ok) {
		throw new Error(data.error || 'Invalid password');
	}
	return data;
}

export async function logoutAuth(): Promise<{ success: boolean }> {
	const res = await fetch('/api/auth/logout', { method: 'POST' });
	if (!res.ok) {
		throw new Error('Failed to logout');
	}
	return res.json();
}

export async function changePasswordAuth(
	currentPassword: string,
	newPassword: string
): Promise<{ success: boolean }> {
	const res = await fetch('/api/auth/change-password', {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ current_password: currentPassword, new_password: newPassword })
	});
	const data = await res.json();
	if (!res.ok) {
		throw new Error(data.error || 'Failed to change password');
	}
	return data;
}
