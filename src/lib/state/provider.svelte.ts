import type { ProviderModel } from '$lib/types';
import {
	fetchProviderModels,
	setModelEnabled,
	setBatchModelsEnabled,
	refreshOpenCodeVersion
} from '$lib/api/providers';

export class ProviderState {
	provider = $state('opencode');
	version = $state('1.19.0');
	models = $state<ProviderModel[]>([]);
	loading = $state(false);
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

	async load() {
		try {
			this.loading = true;
			this.error = null;
			const res = await fetchProviderModels(this.provider);
			this.models = res.models;
			this.version = res.version;
		} catch (e: unknown) {
			this.error = e instanceof Error ? e.message : 'Failed to load provider models';
		} finally {
			this.loading = false;
		}
	}

	async checkVersionUpdate() {
		try {
			const res = await refreshOpenCodeVersion();
			if (res.ok && res.version) {
				this.version = res.version;
			}
			return res;
		} catch (e: unknown) {
			return {
				ok: false,
				version: this.version,
				error: e instanceof Error ? e.message : 'Failed to check version'
			};
		}
	}

	async toggleModel(modelId: string, enabled: boolean) {
		// Optimistic update
		const prev = this.models;
		this.models = this.models.map((m) => (m.id === modelId ? { ...m, enabled } : m));

		try {
			await setModelEnabled(this.provider, modelId, enabled);
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
			await setBatchModelsEnabled(this.provider, targetModels, enabled);
		} catch (e: unknown) {
			this.models = prev;
			throw e;
		}
	}
}
