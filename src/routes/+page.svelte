<script lang="ts">
	import { onMount } from 'svelte';
	import { KeysState } from '$lib/state/keys.svelte';
	import { ProviderState } from '$lib/state/provider.svelte';
	import Sidebar from '$lib/components/sidebar/sidebar.svelte';
	import Navbar from '$lib/components/navbar/navbar.svelte';
	import StatsCards from '$lib/components/keys/stats-cards.svelte';
	import KeysTable from '$lib/components/keys/keys-table.svelte';
	import CreateKeyDialog from '$lib/components/keys/create-key-dialog.svelte';
	import KeySuccessDialog from '$lib/components/keys/key-success-dialog.svelte';
	import OpenCodeProvider from '$lib/components/providers/opencode-provider.svelte';
	import * as Alert from '$lib/components/ui/alert';
	import { Button } from '$lib/components/ui/button';
	import AlertCircleIcon from '@lucide/svelte/icons/alert-circle';
	import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';
	import type { APIKey, CreateKeyPayload } from '$lib/types';

	const keysState = new KeysState();
	const providerState = new ProviderState();

	let activeView = $state<'home' | 'opencode'>('home');
	let mobileMenuOpen = $state(false);

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

	onMount(() => {
		keysState.load();
		providerState.load();
	});
</script>

<div class="flex min-h-screen bg-background text-foreground">
	<!-- Left Navigation Sidebar -->
	<Sidebar
		{activeView}
		onSelectView={(v) => (activeView = v)}
		totalKeys={keysState.totalCount}
		enabledModels={providerState.enabledCount}
		totalModels={providerState.totalCount}
		mobileOpen={mobileMenuOpen}
		onCloseMobile={() => (mobileMenuOpen = false)}
	/>

	<!-- Main Content Area -->
	<div class="flex flex-1 flex-col overflow-x-hidden">
		<Navbar
			error={keysState.error || providerState.error}
			{activeView}
			onToggleMobileMenu={() => (mobileMenuOpen = !mobileMenuOpen)}
		/>

		<main class="mx-auto flex w-full max-w-6xl flex-1 flex-col gap-6 px-4 py-6 sm:px-8">
			{#if keysState.error}
				<Alert.Root variant="destructive">
					<AlertCircleIcon class="size-4" />
					<Alert.Title>Go Backend Unreachable</Alert.Title>
					<Alert.Description class="flex items-center justify-between gap-4">
						<span>{keysState.error}. Please run <code>go run .</code> from the project root.</span>
						<Button
							variant="outline"
							size="sm"
							onclick={() => {
								keysState.load();
								providerState.load();
							}}
						>
							<RefreshCwIcon data-icon="inline-start" />
							Retry
						</Button>
					</Alert.Description>
				</Alert.Root>
			{/if}

			{#if activeView === 'home'}
				<!-- Home Dashboard View -->
				<StatsCards totalCount={keysState.totalCount} activeCount={keysState.activeCount} />

				<!-- API Keys Management -->
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
			{:else}
				<!-- OpenCode Free Provider View (9router style empty prompt model testing + enable/disable) -->
				<OpenCodeProvider {providerState} />
			{/if}
		</main>
	</div>
</div>

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
