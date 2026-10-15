import { fetchAuthStatus, loginAuth, setupAuth, logoutAuth } from '$lib/api/auth';

export class AuthState {
	authEnabled = $state(false);
	setupRequired = $state(false);
	authenticated = $state(false);
	loading = $state(true);
	submitting = $state(false);
	error = $state<string | null>(null);

	async checkStatus() {
		this.loading = true;
		this.error = null;
		try {
			const status = await fetchAuthStatus();
			this.authEnabled = status.auth_enabled;
			this.setupRequired = status.setup_required;
			this.authenticated = status.authenticated;
		} catch (err: unknown) {
			this.error = (err as Error).message || 'Failed to check authentication status';
		} finally {
			this.loading = false;
		}
	}

	async login(password: string): Promise<boolean> {
		this.submitting = true;
		this.error = null;
		try {
			const res = await loginAuth(password);
			if (res.success) {
				this.authenticated = true;
				this.setupRequired = false;
				return true;
			}
			return false;
		} catch (err: unknown) {
			this.error = (err as Error).message || 'Invalid password';
			return false;
		} finally {
			this.submitting = false;
		}
	}

	async setup(password: string): Promise<boolean> {
		this.submitting = true;
		this.error = null;
		try {
			const res = await setupAuth(password);
			if (res.success) {
				this.authEnabled = true;
				this.setupRequired = false;
				this.authenticated = true;
				return true;
			}
			return false;
		} catch (err: unknown) {
			this.error = (err as Error).message || 'Failed to setup password';
			return false;
		} finally {
			this.submitting = false;
		}
	}

	async logout() {
		try {
			await logoutAuth();
		} catch {
			// Ignore error on logout
		}
		this.authenticated = false;
		await this.checkStatus();
	}
}

export const authState = new AuthState();
