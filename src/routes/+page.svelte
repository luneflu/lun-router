<script lang="ts">
	import { keysState } from '$lib/state/keys.svelte';
	import StatsCards from '$lib/components/keys/stats-cards.svelte';
	import KeysTable from '$lib/components/keys/keys-table.svelte';
	import CreateKeyDialog from '$lib/components/keys/create-key-dialog.svelte';
	import KeySuccessDialog from '$lib/components/keys/key-success-dialog.svelte';
	import type { APIKey, CreateKeyPayload } from '$lib/types';

	let isCreateOpen = $state(false);
	let isSuccessOpen = $state(false);
	let justCreatedKey = $state<APIKey | null>(null);

	async function handleCreateKey(payload: CreateKeyPayload) {
		const newKey = await keysState.create(payload);
		justCreatedKey = newKey;
		isSuccessOpen = true;
	}

	async function handleToggleActive(key: APIKey, checked: boolean) {
		try {
			await keysState.toggleActive(key, checked);
		} catch (e: unknown) {
			console.error('Failed to toggle active state', e);
		}
	}

	async function handleDeleteKey(id: string) {
		if (!confirm('Are you sure you want to delete this API key? This action cannot be undone.')) {
			return;
		}
		try {
			await keysState.remove(id);
		} catch (e: unknown) {
			console.error('Failed to delete key', e);
		}
	}
</script>

<StatsCards totalCount={keysState.totalCount} activeCount={keysState.activeCount} />

<KeysTable
	keys={keysState.filtered}
	totalCount={keysState.totalCount}
	loading={keysState.loading}
	searchQuery={keysState.searchQuery}
	onSearchChange={(q) => (keysState.searchQuery = q)}
	onCreateClick={() => (isCreateOpen = true)}
	onToggleActive={handleToggleActive}
	onDeleteKey={handleDeleteKey}
/>

<CreateKeyDialog
	bind:open={isCreateOpen}
	primaryMachineId={keysState.primaryMachineId}
	onCreate={handleCreateKey}
	onClose={() => (isCreateOpen = false)}
/>

<KeySuccessDialog
	bind:open={isSuccessOpen}
	keyData={justCreatedKey}
	onClose={() => (isSuccessOpen = false)}
/>
