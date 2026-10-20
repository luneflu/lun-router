import type { CommandCodeAccount, ProviderModel } from '$lib/types';
import {
	fetchCommandCodeAccounts,
	createCommandCodeAccount,
	setCommandCodeAccountActive,
	deleteCommandCodeAccount,
	fetchCommandCodeModels,
	refreshCommandCodeModels,
	setCommandCodeModelEnabled,
	setCommandCodeBatchModelsEnabled,
	testCommandCodeAccount,
	type CreateAccountPayload
} from '$lib/api/commandcode';
import {
	addCustomModel as apiAddCustomModel,
	deleteCustomModel as apiDeleteCustomModel,
	type CreateCustomModelPayload
} from '$lib/api/providers';

export class CommandCodeState {
	provider = 'commandcode';
	accounts = $state<CommandCodeAccount[]>([]);
	models = $state<ProviderModel[]>([]);
	loading = $state(false);
	accountsLoading = $state(false);
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
			await Promise.all([this.loadModels(), this.loadAccounts()]);
		} catch (e: unknown) {
			this.error = e instanceof Error ? e.message : 'Failed to load Command Code provider';
		} finally {
			this.loading = false;
		}
	}

	async loadModels() {
		this.models = await fetchCommandCodeModels();
	}

	async syncModels() {
		try {
			const res = await refreshCommandCodeModels();
			if (res.ok) {
				await this.loadModels();
				return { ok: true, count: res.count, error: undefined };
			}
			return { ok: false, count: this.models.length, error: res.error || 'Failed to sync models' };
		} catch (e: unknown) {
			return {
				ok: false,
				count: this.models.length,
				error: e instanceof Error ? e.message : 'Failed to sync models'
			};
		}
	}

	async loadAccounts() {
		try {
			this.accountsLoading = true;
			this.accounts = await fetchCommandCodeAccounts();
		} finally {
			this.accountsLoading = false;
		}
	}

	async addAccount(payload: CreateAccountPayload) {
		const newAcc = await createCommandCodeAccount(payload);
		this.accounts = [newAcc, ...this.accounts.filter((a) => a.id !== newAcc.id)];
		return newAcc;
	}

	async toggleAccount(id: string, active: boolean) {
		const prev = this.accounts;
		this.accounts = this.accounts.map((a) => (a.id === id ? { ...a, isActive: active } : a));
		try {
			await setCommandCodeAccountActive(id, active);
		} catch (e: unknown) {
			this.accounts = prev;
			throw e;
		}
	}

	async deleteAccount(id: string) {
		const prev = this.accounts;
		this.accounts = this.accounts.filter((a) => a.id !== id);
		try {
			await deleteCommandCodeAccount(id);
		} catch (e: unknown) {
			this.accounts = prev;
			throw e;
		}
	}

	async testAccount(id: string) {
		return await testCommandCodeAccount(id);
	}

	async toggleModel(modelId: string, enabled: boolean) {
		const prev = this.models;
		this.models = this.models.map((m) => (m.id === modelId ? { ...m, enabled } : m));
		try {
			await setCommandCodeModelEnabled(modelId, enabled);
		} catch (e: unknown) {
			this.models = prev;
			throw e;
		}
	}

	async toggleAll(enabled: boolean) {
		const prev = this.models;
		this.models = this.models.map((m) => ({ ...m, enabled }));
		try {
			await setCommandCodeBatchModelsEnabled(
				prev.map((m) => m.id),
				enabled
			);
		} catch (e: unknown) {
			this.models = prev;
			throw e;
		}
	}

	async addCustomModel(payload: CreateCustomModelPayload) {
		const res = await apiAddCustomModel(this.provider, payload);
		await this.load();
		return res;
	}

	async deleteCustomModel(modelId: string) {
		const res = await apiDeleteCustomModel(this.provider, modelId);
		this.models = this.models.filter((m) => m.id !== modelId && m.id !== `commandcode/${modelId}`);
		return res;
	}
}

export const commandCodeState = new CommandCodeState();
