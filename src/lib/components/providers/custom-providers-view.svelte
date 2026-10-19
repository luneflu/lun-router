<script lang="ts">
	import * as Card from '$lib/components/ui/card';
	import { Button } from '$lib/components/ui/button';
	import { Badge } from '$lib/components/ui/badge';
	import { Switch } from '$lib/components/ui/switch';
	import { toast } from '$lib/state/toast.svelte';
	import { customProvidersState } from '$lib/state/custom-providers.svelte';
	import type {
		CustomProvider,
		CreateCustomProviderPayload,
		UpdateCustomProviderPayload,
		ProviderModel
	} from '$lib/types';
	import ProviderDialog from './custom/provider-dialog.svelte';
	import ModelListCard from './model-list-card.svelte';
	import ModelTestChat from './model-test-chat.svelte';

	import ServerIcon from '@lucide/svelte/icons/server';
	import PlusIcon from '@lucide/svelte/icons/plus';
	import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';
	import PencilIcon from '@lucide/svelte/icons/pencil';
	import Trash2Icon from '@lucide/svelte/icons/trash-2';
	import KeyRoundIcon from '@lucide/svelte/icons/key-round';
	import ExternalLinkIcon from '@lucide/svelte/icons/external-link';

	let isDialogOpen = $state(false);
	let editingProvider = $state<CustomProvider | null>(null);
	let syncing = $state(false);
	let selectedChatModel = $state('');

	let activeProvider = $derived(customProvidersState.selectedProvider);
	let providerModels = $derived<ProviderModel[]>(
		activeProvider ? customProvidersState.models[activeProvider.id] || [] : []
	);

	function openAddDialog() {
		editingProvider = null;
		isDialogOpen = true;
	}

	function openEditDialog(provider: CustomProvider) {
		editingProvider = provider;
		isDialogOpen = true;
	}

	async function handleSaveProvider(
		payload: CreateCustomProviderPayload | UpdateCustomProviderPayload
	) {
		if (editingProvider) {
			await customProvidersState.update(editingProvider.id, payload as UpdateCustomProviderPayload);
			toast.success(
				'Provider Updated',
				`${payload.name || editingProvider.name} has been updated.`
			);
		} else {
			const created = await customProvidersState.create(payload as CreateCustomProviderPayload);
			toast.success('Provider Created', `${created.name} (${created.prefix}) is ready.`);
		}
	}

	async function handleDeleteProvider(provider: CustomProvider) {
		if (
			!confirm(
				`Are you sure you want to delete "${provider.name}" (${provider.prefix})? This action cannot be undone.`
			)
		) {
			return;
		}
		try {
			await customProvidersState.remove(provider.id);
			toast.success('Provider Deleted', `Provider "${provider.name}" removed.`);
		} catch (e: unknown) {
			toast.error('Delete Failed', e instanceof Error ? e.message : 'Failed to delete provider');
		}
	}

	async function handleToggleEnabled(provider: CustomProvider, checked: boolean) {
		try {
			await customProvidersState.toggleEnabled(provider.id, checked);
			toast.success(
				'Provider Updated',
				`${provider.name} is now ${checked ? 'enabled' : 'disabled'}.`
			);
		} catch (e: unknown) {
			toast.error('Update Failed', e instanceof Error ? e.message : 'Failed to toggle provider');
		}
	}

	async function handleSyncModels() {
		if (!activeProvider) return;
		try {
			syncing = true;
			const res = await customProvidersState.syncModels(activeProvider.id);
			if (res.ok) {
				toast.success('Models Synced', `Found ${res.count} models from ${activeProvider.name}`);
			} else {
				toast.error('Sync Failed', 'Failed to retrieve models from upstream.');
			}
		} catch (e: unknown) {
			toast.error('Sync Failed', e instanceof Error ? e.message : 'Error syncing models');
		} finally {
			syncing = false;
		}
	}

	async function handleToggleModel(model: ProviderModel, enabled: boolean) {
		if (!activeProvider) return;
		try {
			await customProvidersState.toggleModel(activeProvider.id, model, enabled);
		} catch (e: unknown) {
			toast.error('Update Failed', e instanceof Error ? e.message : 'Failed to update model');
		}
	}

	async function handleToggleAllModels(enabled: boolean) {
		if (!activeProvider) return;
		try {
			await customProvidersState.toggleAllModels(activeProvider.id, enabled);
			toast.success('Models Updated', `All models ${enabled ? 'enabled' : 'disabled'}`);
		} catch (e: unknown) {
			toast.error('Update Failed', e instanceof Error ? e.message : 'Failed to update models');
		}
	}

	async function handleAddModel(data: {
		id: string;
		context_length?: number;
		max_completion_tokens?: number;
		vision?: boolean;
		reasoning?: boolean;
		tools?: boolean;
		thinking_levels?: string[];
	}) {
		if (!activeProvider) return;
		try {
			await customProvidersState.addModel(activeProvider.id, data);
			toast.success('Model Added', `Custom model ${data.id} added.`);
		} catch (e: unknown) {
			toast.error('Failed to Add Model', e instanceof Error ? e.message : 'Error adding model');
			throw e;
		}
	}

	async function handleDeleteModel(modelId: string) {
		if (!activeProvider) return;
		try {
			await customProvidersState.deleteModel(activeProvider.id, modelId);
			toast.success('Model Deleted', `Model removed.`);
		} catch (e: unknown) {
			toast.error('Failed to Delete', e instanceof Error ? e.message : 'Error deleting model');
			throw e;
		}
	}
</script>

<div class="flex flex-col gap-6">
	<!-- Overview Header Card -->
	<Card.Root class="overflow-hidden p-0">
		<div class="border-b border-border bg-muted/20 p-6 sm:p-8">
			<div class="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
				<div class="flex items-start gap-4">
					<div
						class="flex size-14 shrink-0 items-center justify-center rounded-2xl border border-primary/20 bg-primary/10 text-primary shadow-xs"
					>
						<ServerIcon class="size-7" />
					</div>
					<div class="flex flex-col gap-1.5">
						<div class="flex flex-wrap items-center gap-2.5">
							<h2 class="text-xl font-bold tracking-tight text-foreground">Custom Providers</h2>
							<Badge
								variant="outline"
								class="border-primary/30 bg-primary/10 font-medium text-primary"
							>
								OpenAI Compatible
							</Badge>
							{#if customProvidersState.activeCount > 0}
								<Badge
									variant="outline"
									class="border-emerald-500/30 bg-emerald-500/10 font-medium text-emerald-600 dark:text-emerald-400"
								>
									{customProvidersState.activeCount} Active
								</Badge>
							{/if}
						</div>
						<p class="max-w-2xl text-xs leading-relaxed text-muted-foreground">
							Connect any OpenAI-compatible provider (Groq, DeepSeek, Together, Ollama, vLLM, LM
							Studio, etc.) by setting a custom name and routing prefix.
						</p>
					</div>
				</div>

				<div class="flex shrink-0 items-center gap-2">
					<Button size="sm" class="gap-1.5 text-xs" onclick={openAddDialog}>
						<PlusIcon class="size-3.5" />
						<span>Add Provider</span>
					</Button>
				</div>
			</div>
		</div>

		<!-- Metric stats -->
		<Card.Content class="grid grid-cols-2 divide-x divide-border p-0 sm:grid-cols-3">
			<div class="flex flex-col gap-1 p-4 sm:p-5">
				<span class="text-[11px] font-medium text-muted-foreground">Total Providers</span>
				<div class="flex items-baseline gap-2">
					<span class="text-2xl font-bold tracking-tight text-foreground"
						>{customProvidersState.totalCount}</span
					>
					<span class="text-xs text-muted-foreground">configured</span>
				</div>
			</div>

			<div class="flex flex-col gap-1 p-4 sm:p-5">
				<span class="text-[11px] font-medium text-muted-foreground">Active Providers</span>
				<div class="flex items-baseline gap-2">
					<span class="text-2xl font-bold tracking-tight text-emerald-600 dark:text-emerald-400">
						{customProvidersState.activeCount}
					</span>
					<span class="text-xs text-muted-foreground">routing enabled</span>
				</div>
			</div>

			<div class="flex flex-col gap-1 p-4 sm:p-5">
				<span class="text-[11px] font-medium text-muted-foreground">Total Models</span>
				<div class="flex items-baseline gap-2">
					<span class="text-2xl font-bold tracking-tight text-foreground"
						>{customProvidersState.totalModels}</span
					>
					<span class="text-xs text-muted-foreground">available</span>
				</div>
			</div>
		</Card.Content>
	</Card.Root>

	<!-- Providers Tabs / Selector -->
	{#if customProvidersState.providers.length === 0}
		<Card.Root class="p-8 text-center sm:p-12">
			<div
				class="mx-auto flex size-12 items-center justify-center rounded-xl border border-dashed border-border bg-muted/40 text-muted-foreground"
			>
				<ServerIcon class="size-6" />
			</div>
			<h3 class="mt-4 text-sm font-semibold text-foreground">No Custom Providers Yet</h3>
			<p class="mx-auto mt-1.5 max-w-md text-xs leading-relaxed text-muted-foreground">
				Add an OpenAI-compatible upstream endpoint to use models from Groq, DeepSeek, Together AI,
				or local instances like Ollama and LM Studio.
			</p>
			<div class="mt-6">
				<Button size="sm" onclick={openAddDialog} class="gap-1.5 text-xs">
					<PlusIcon class="size-3.5" />
					<span>Add First Provider</span>
				</Button>
			</div>
		</Card.Root>
	{:else}
		<!-- Provider Selector Row -->
		<div class="flex flex-wrap items-center gap-2">
			{#each customProvidersState.providers as prov (prov.id)}
				<button
					type="button"
					onclick={() => customProvidersState.selectProvider(prov.id)}
					class={`flex items-center gap-2 rounded-lg border px-3 py-2 text-xs font-medium transition-colors ${
						activeProvider?.id === prov.id
							? 'border-primary bg-primary/10 text-primary shadow-xs'
							: 'border-border bg-card text-muted-foreground hover:bg-muted hover:text-foreground'
					}`}
				>
					<span class="font-semibold">{prov.name}</span>
					<Badge variant="outline" class="font-mono text-[10px]">
						{prov.prefix}/
					</Badge>
					<span
						class={`size-2 rounded-full ${prov.isEnabled ? 'bg-emerald-500' : 'bg-muted-foreground/40'}`}
						title={prov.isEnabled ? 'Enabled' : 'Disabled'}
					></span>
				</button>
			{/each}
		</div>

		{#if activeProvider}
			<!-- Active Provider Details & Actions -->
			<Card.Root class="p-5 sm:p-6">
				<div class="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
					<div class="flex flex-col gap-1.5">
						<div class="flex flex-wrap items-center gap-2">
							<h3 class="text-lg font-bold tracking-tight text-foreground">
								{activeProvider.name}
							</h3>
							<Badge variant="outline" class="font-mono text-xs">
								Prefix: {activeProvider.prefix}/
							</Badge>
							<Badge
								variant="outline"
								class={activeProvider.isEnabled
									? 'border-emerald-500/30 bg-emerald-500/10 text-emerald-600 dark:text-emerald-400'
									: 'text-muted-foreground'}
							>
								{activeProvider.isEnabled ? 'Active' : 'Disabled'}
							</Badge>
						</div>

						<div class="flex flex-wrap items-center gap-4 text-xs text-muted-foreground">
							<div class="flex items-center gap-1.5 font-mono text-[11px]">
								<ExternalLinkIcon class="size-3" />
								<span>{activeProvider.baseUrl}</span>
							</div>
							<div class="flex items-center gap-1.5">
								<KeyRoundIcon class="size-3" />
								<span
									>{activeProvider.hasApiKey
										? activeProvider.apiKeyPreview || 'Key configured'
										: 'No key (local/open)'}</span
								>
							</div>
						</div>
					</div>

					<div class="flex flex-wrap items-center gap-2">
						<div class="flex items-center gap-2 border-e border-border pe-2">
							<span class="text-xs text-muted-foreground">Enabled</span>
							<Switch
								checked={activeProvider.isEnabled}
								onCheckedChange={(checked) =>
									activeProvider && handleToggleEnabled(activeProvider, checked)}
							/>
						</div>
						<Button
							variant="outline"
							size="sm"
							class="gap-1.5 text-xs"
							onclick={handleSyncModels}
							disabled={syncing}
						>
							<RefreshCwIcon class={`size-3.5 ${syncing ? 'animate-spin' : ''}`} />
							<span>Sync Models</span>
						</Button>
						<Button
							variant="outline"
							size="sm"
							class="gap-1.5 text-xs"
							onclick={() => activeProvider && openEditDialog(activeProvider)}
						>
							<PencilIcon class="size-3.5" />
							<span>Edit</span>
						</Button>
						<Button
							variant="destructive"
							size="sm"
							class="gap-1.5 text-xs"
							onclick={() => activeProvider && handleDeleteProvider(activeProvider)}
						>
							<Trash2Icon class="size-3.5" />
							<span>Delete</span>
						</Button>
					</div>
				</div>
			</Card.Root>

			<!-- Test Chat Card -->
			{#if providerModels.length > 0}
				<ModelTestChat models={providerModels} bind:selectedModelId={selectedChatModel} />
			{/if}

			<!-- Models Card -->
			<ModelListCard
				title={`${activeProvider.name} Models (${activeProvider.prefix}/)`}
				description={`Models available through "${activeProvider.prefix}/<model>" prefix routing. Probe ping sends requests directly to ${activeProvider.baseUrl}.`}
				models={providerModels}
				loading={customProvidersState.modelsLoading[activeProvider.id] || false}
				onToggleModel={handleToggleModel}
				onToggleAll={handleToggleAllModels}
				onSelectChatModel={(id) => (selectedChatModel = id)}
				onAddModel={handleAddModel}
				onDeleteModel={handleDeleteModel}
			/>
		{/if}
	{/if}
</div>

<ProviderDialog
	bind:open={isDialogOpen}
	provider={editingProvider}
	onSave={handleSaveProvider}
	onClose={() => {
		isDialogOpen = false;
		editingProvider = null;
	}}
/>
