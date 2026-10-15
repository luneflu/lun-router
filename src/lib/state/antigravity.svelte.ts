import type { AntigravityAccount, ProviderModel, AntigravityAccountUsage } from '$lib/types';
import {
	fetchAntigravityAccounts,
	createAntigravityAccount,
	setAntigravityAccountActive,
	deleteAntigravityAccount,
	refreshAntigravityAccount,
	fetchAntigravityModels,
	refreshAntigravityModels,
	setAntigravityModelEnabled,
	setAntigravityBatchModelsEnabled,
	fetchAntigravityQuota,
	type CreateAccountPayload
} from '$lib/api/antigravity';

export class AntigravityState {
	provider = 'antigravity';
	version = $state('2.11.0');
	accounts = $state<AntigravityAccount[]>([]);
	models = $state<ProviderModel[]>([]);
	usages = $state<Record<string, AntigravityAccountUsage>>({});
	loading = $state(false);
	accountsLoading = $state(false);
	quotaLoading = $state(false);
	error = $state<string | null>(null);
	searchQuery = $state('');
	filter = $state<'all' | 'enabled' | 'disabled'>('all');

	filtered = $derived(
		this.models.filter((m) => {
			const matchesQuery = m.id.toLowerCase().includes(this.searchQuery.toLowerCase());
			if (!matchesQuery) return false;
			if (this.filter === 'enabled') return m.enabled;
			if (this.filter === 'disabled') return !m.enabled;
			return true;
		})
	);

	totalCount = $derived(this.models.length);
	enabledCount = $derived(this.models.filter((m) => m.enabled).length);
	disabledCount = $derived(this.models.filter((m) => !m.enabled).length);
	totalAccountsCount = $derived(this.accounts.length);
	activeAccountsCount = $derived(this.accounts.filter((a) => a.isActive).length);

	async load() {
		try {
			this.loading = true;
			this.error = null;
			await Promise.all([this.loadModels(), this.loadAccounts(), this.loadQuota()]);
		} catch (e: unknown) {
			this.error = e instanceof Error ? e.message : 'Failed to load Antigravity provider';
		} finally {
			this.loading = false;
		}
	}

	async loadQuota(accountId?: string, force = false) {
		try {
			this.quotaLoading = true;
			const data = await fetchAntigravityQuota(accountId, force);
			this.usages = { ...this.usages, ...data };
		} catch (e: unknown) {
			// Soft-fail for quota: logging or keeping previous cache
			console.warn('Failed to load quota:', e);
		} finally {
			this.quotaLoading = false;
		}
	}

	async loadModels() {
		const res = await fetchAntigravityModels();
		this.models = res.models;
		this.version = res.version;
	}

	async syncModels() {
		try {
			const res = await refreshAntigravityModels();
			if (res.ok) {
				await this.loadModels();
			}
			return res;
		} catch (e: unknown) {
			return {
				ok: false,
				count: this.models.length,
				updated: false,
				error: e instanceof Error ? e.message : 'Failed to sync models'
			};
		}
	}

	async loadAccounts() {
		try {
			this.accountsLoading = true;
			this.accounts = await fetchAntigravityAccounts();
		} finally {
			this.accountsLoading = false;
		}
	}

	async addAccount(payload: CreateAccountPayload) {
		const newAcc = await createAntigravityAccount(payload);
		this.accounts = [newAcc, ...this.accounts.filter((a) => a.id !== newAcc.id)];
		return newAcc;
	}

	async toggleAccount(id: string, active: boolean) {
		const prev = this.accounts;
		this.accounts = this.accounts.map((a) => (a.id === id ? { ...a, isActive: active } : a));
		try {
			await setAntigravityAccountActive(id, active);
		} catch (e: unknown) {
			this.accounts = prev;
			throw e;
		}
	}

	async deleteAccount(id: string) {
		const prev = this.accounts;
		this.accounts = this.accounts.filter((a) => a.id !== id);
		try {
			await deleteAntigravityAccount(id);
		} catch (e: unknown) {
			this.accounts = prev;
			throw e;
		}
	}

	async refreshAccount(id: string) {
		const res = await refreshAntigravityAccount(id);
		if (res.ok && res.expiresAt) {
			this.accounts = this.accounts.map((a) =>
				a.id === id ? { ...a, expiresAt: res.expiresAt } : a
			);
		}
		return res;
	}

	async toggleModel(modelId: string, enabled: boolean) {
		const prev = this.models;
		this.models = this.models.map((m) => (m.id === modelId ? { ...m, enabled } : m));
		try {
			await setAntigravityModelEnabled(modelId, enabled);
		} catch (e: unknown) {
			this.models = prev;
			throw e;
		}
	}

	async toggleAll(enabled: boolean) {
		const targetModels = this.models.map((m) => m.id);
		const prev = this.models;
		this.models = this.models.map((m) => ({ ...m, enabled }));
		try {
			await setAntigravityBatchModelsEnabled(targetModels, enabled);
		} catch (e: unknown) {
			this.models = prev;
			throw e;
		}
	}
}

export const antigravityState = new AntigravityState();
