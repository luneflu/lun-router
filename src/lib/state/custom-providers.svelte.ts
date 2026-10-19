import type {
	CustomProvider,
	CreateCustomProviderPayload,
	UpdateCustomProviderPayload,
	ProviderModel
} from '$lib/types';
import {
	fetchCustomProviders,
	createCustomProvider,
	updateCustomProvider,
	deleteCustomProvider,
	syncCustomProviderModels,
	fetchCustomProviderModels,
	updateCustomProviderModels,
	addCustomProviderModel,
	deleteCustomProviderModel
} from '$lib/api/custom-providers';
import type { CreateCustomModelPayload } from '$lib/api/providers';

export class CustomProvidersState {
	providers = $state<CustomProvider[]>([]);
	selectedId = $state<string | null>(null);
	models = $state<Record<string, ProviderModel[]>>({});
	modelsLoading = $state<Record<string, boolean>>({});
	loading = $state(false);
	error = $state<string | null>(null);

	totalCount = $derived(this.providers.length);
	activeCount = $derived(this.providers.filter((p) => p.isEnabled).length);
	totalModels = $derived(
		this.providers.reduce((acc, p) => acc + (p.modelCount || (p.models ? p.models.length : 0)), 0)
	);

	selectedProvider = $derived(
		this.providers.find((p) => p.id === this.selectedId) || this.providers[0] || null
	);

	async load() {
		try {
			this.loading = true;
			this.error = null;
			this.providers = await fetchCustomProviders();
			if (this.providers.length > 0 && !this.selectedId) {
				this.selectedId = this.providers[0].id;
			}
			if (this.selectedId) {
				await this.loadModels(this.selectedId);
			}
		} catch (e: unknown) {
			this.error = e instanceof Error ? e.message : 'Failed to load custom providers';
		} finally {
			this.loading = false;
		}
	}

	async selectProvider(id: string) {
		this.selectedId = id;
		if (!this.models[id]) {
			await this.loadModels(id);
		}
	}

	async loadModels(id: string) {
		try {
			this.modelsLoading[id] = true;
			const list = await fetchCustomProviderModels(id);
			this.models[id] = list;
		} catch (e: unknown) {
			console.error(`Failed to load models for provider ${id}`, e);
		} finally {
			this.modelsLoading[id] = false;
		}
	}

	async create(payload: CreateCustomProviderPayload): Promise<CustomProvider> {
		const created = await createCustomProvider(payload);
		this.providers = [...this.providers, created];
		this.selectedId = created.id;
		// Auto sync if user provided baseUrl
		try {
			await this.syncModels(created.id);
		} catch (e) {
			console.warn('Initial model sync error:', e);
		}
		return created;
	}

	async update(id: string, payload: UpdateCustomProviderPayload): Promise<CustomProvider> {
		const updated = await updateCustomProvider(id, payload);
		this.providers = this.providers.map((p) => (p.id === id ? updated : p));
		return updated;
	}

	async toggleEnabled(id: string, enabled: boolean): Promise<void> {
		const prev = this.providers;
		this.providers = this.providers.map((p) => (p.id === id ? { ...p, isEnabled: enabled } : p));
		try {
			await updateCustomProvider(id, { isEnabled: enabled });
		} catch (e) {
			this.providers = prev;
			throw e;
		}
	}

	async remove(id: string): Promise<void> {
		await deleteCustomProvider(id);
		this.providers = this.providers.filter((p) => p.id !== id);
		const newModels = { ...this.models };
		delete newModels[id];
		this.models = newModels;
		if (this.selectedId === id) {
			this.selectedId = this.providers[0]?.id || null;
			if (this.selectedId) {
				await this.loadModels(this.selectedId);
			}
		}
	}

	async syncModels(id: string) {
		const res = await syncCustomProviderModels(id);
		await this.loadModels(id);
		// Update model count in provider item
		this.providers = this.providers.map((p) =>
			p.id === id ? { ...p, modelCount: res.count, models: res.models } : p
		);
		return res;
	}

	async toggleModel(id: string, model: ProviderModel, enabled: boolean) {
		const providerModels = this.models[id] || [];
		const prev = providerModels;
		this.models[id] = providerModels.map((m) => (m.id === model.id ? { ...m, enabled } : m));

		try {
			await updateCustomProviderModels(id, { model: model.id, enabled });
		} catch (e) {
			this.models[id] = prev;
			throw e;
		}
	}

	async toggleAllModels(id: string, enabled: boolean) {
		const providerModels = this.models[id] || [];
		const prev = providerModels;
		this.models[id] = providerModels.map((m) => ({ ...m, enabled }));

		try {
			const modelIds = providerModels.map((m) => m.id);
			await updateCustomProviderModels(id, { models: modelIds, enabled });
		} catch (e) {
			this.models[id] = prev;
			throw e;
		}
	}

	async addModel(id: string, payload: CreateCustomModelPayload) {
		const newModel = await addCustomProviderModel(id, payload);
		const existing = this.models[id] || [];
		this.models[id] = [...existing.filter((m) => m.id !== newModel.id), newModel];
		return newModel;
	}

	async deleteModel(id: string, modelId: string) {
		await deleteCustomProviderModel(id, modelId);
		const existing = this.models[id] || [];
		this.models[id] = existing.filter((m) => m.id !== modelId);
	}
}

export const customProvidersState = new CustomProvidersState();
