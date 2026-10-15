import type { APIKey, CreateKeyPayload } from '$lib/types';
import { fetchApiKeys, createApiKey, updateApiKeyActive, deleteApiKey } from '$lib/api/keys';

export class KeysState {
	items = $state<APIKey[]>([]);
	loading = $state(true);
	error = $state<string | null>(null);
	searchQuery = $state('');

	filtered = $derived(
		this.items.filter(
			(k) =>
				k.name.toLowerCase().includes(this.searchQuery.toLowerCase()) ||
				k.key.toLowerCase().includes(this.searchQuery.toLowerCase())
		)
	);

	totalCount = $derived(this.items.length);
	activeCount = $derived(this.items.filter((k) => k.isActive).length);
	primaryMachineId = $derived(this.items.find((k) => k.machineId)?.machineId || 'local-host');

	async load() {
		try {
			this.loading = true;
			this.error = null;
			this.items = await fetchApiKeys();
		} catch (e: unknown) {
			this.error = e instanceof Error ? e.message : 'Go backend unreachable on :20128';
		} finally {
			this.loading = false;
		}
	}

	async create(payload: CreateKeyPayload): Promise<APIKey> {
		const created = await createApiKey(payload);
		await this.load();
		return created;
	}

	async toggleActive(key: APIKey, checked: boolean) {
		await updateApiKeyActive(key.id, checked);
		this.items = this.items.map((k) => (k.id === key.id ? { ...k, isActive: checked } : k));
	}

	async remove(id: string) {
		await deleteApiKey(id);
		this.items = this.items.filter((k) => k.id !== id);
	}
}

export const keysState = new KeysState();
