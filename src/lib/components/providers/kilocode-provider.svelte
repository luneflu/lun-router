<script lang="ts">
	import * as Card from '$lib/components/ui/card';
	import { Button } from '$lib/components/ui/button';
	import { Badge } from '$lib/components/ui/badge';
	import { KilocodeState } from '$lib/state/kilocode.svelte';
	import { toast } from '$lib/state/toast.svelte';
	import ModelTestChat from './model-test-chat.svelte';
	import ModelListCard from './model-list-card.svelte';
	import AccountsCard from './kilocode/accounts-card.svelte';

	import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';

	interface Props {
		kilocodeState: KilocodeState;
	}

	let { kilocodeState }: Props = $props();

	let selectedChatModel = $state('anthropic/claude-sonnet-4-20250514');
	let syncingModels = $state(false);

	async function handleSyncModels() {
		try {
			syncingModels = true;
			const res = await kilocodeState.syncModels();
			if (res.ok) {
				toast.success('Models Synced', `Up to date: ${res.count} models available`);
			} else {
				toast.error('Sync Failed', res.error || 'Failed to sync models');
			}
		} finally {
			syncingModels = false;
		}
	}
</script>

<div class="flex flex-col gap-6">
	<!-- Provider Overview Header Card -->
	<Card.Root class="overflow-hidden p-0">
		<div class="border-b border-border bg-muted/20 p-6 sm:p-8">
			<div class="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
				<div class="flex items-start gap-4">
					<div
						class="flex size-14 shrink-0 items-center justify-center rounded-2xl border border-orange-500/20 bg-orange-500/10 text-orange-500 shadow-xs"
					>
						<img src="/providers/kilocode.png" alt="Kilo Code" class="size-8 object-contain" />
					</div>
					<div class="flex flex-col gap-1.5">
						<div class="flex flex-wrap items-center gap-2.5">
							<h2 class="text-xl font-bold tracking-tight text-foreground">Kilo Code</h2>
							<Badge variant="outline" class="border-orange-500/30 bg-orange-500/10 text-orange-600 dark:text-orange-400 font-medium">
								OpenRouter Gateway
							</Badge>
							{#if kilocodeState.activeAccountsCount > 0}
								<Badge variant="outline" class="border-emerald-500/30 bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 font-medium">
									Ready ({kilocodeState.activeAccountsCount} active)
								</Badge>
							{:else}
								<Badge variant="secondary" class="font-medium text-muted-foreground">
									No Accounts Connected
								</Badge>
							{/if}
						</div>
						<p class="max-w-2xl text-xs leading-relaxed text-muted-foreground">
							Direct access to Kilo Code's multi-model AI gateway (powered by OpenRouter) with device code OAuth authentication. Supports Claude, Gemini, GPT-4, DeepSeek, and community free models.
						</p>
					</div>
				</div>

				<div class="flex items-center gap-2 shrink-0">
					<Button
						variant="outline"
						size="sm"
						class="gap-1.5 text-xs"
						onclick={handleSyncModels}
						disabled={syncingModels}
					>
						<RefreshCwIcon class={`size-3.5 ${syncingModels ? 'animate-spin' : ''}`} />
						<span>Sync Models</span>
					</Button>
				</div>
			</div>
		</div>

		<!-- Quick Metric Stats -->
		<Card.Content class="grid grid-cols-2 divide-x divide-border p-0 sm:grid-cols-4">
			<div class="flex flex-col gap-1 p-4 sm:p-5">
				<span class="text-[11px] font-medium text-muted-foreground">Total Models</span>
				<div class="flex items-baseline gap-2">
					<span class="text-2xl font-bold tracking-tight text-foreground">{kilocodeState.totalCount}</span>
					<span class="text-xs text-muted-foreground">available</span>
				</div>
			</div>

			<div class="flex flex-col gap-1 p-4 sm:p-5">
				<span class="text-[11px] font-medium text-muted-foreground">Enabled Models</span>
				<div class="flex items-baseline gap-2">
					<span class="text-2xl font-bold tracking-tight text-emerald-600 dark:text-emerald-400">
						{kilocodeState.enabledCount}
					</span>
					<span class="text-xs text-muted-foreground">active</span>
				</div>
			</div>

			<div class="flex flex-col gap-1 p-4 sm:p-5">
				<span class="text-[11px] font-medium text-muted-foreground">Active Accounts</span>
				<div class="flex items-baseline gap-2">
					<span class="text-2xl font-bold tracking-tight text-foreground">
						{kilocodeState.activeAccountsCount}
					</span>
					<span class="text-xs text-muted-foreground">/ {kilocodeState.totalAccountsCount} total</span>
				</div>
			</div>

			<div class="flex flex-col gap-1 p-4 sm:p-5">
				<span class="text-[11px] font-medium text-muted-foreground">Auth Flow</span>
				<div class="flex items-baseline gap-2">
					<span class="text-base font-bold text-foreground">Device Code</span>
					<span class="text-[11px] text-muted-foreground">OAuth 2.0</span>
				</div>
			</div>
		</Card.Content>
	</Card.Root>

	<!-- Accounts Management Section -->
	<AccountsCard {kilocodeState} />

	<!-- Interactive Test Chat -->
	<ModelTestChat
		bind:selectedModelId={selectedChatModel}
		models={kilocodeState.models}
	/>

	<!-- Models Catalog Card -->
	<ModelListCard
		title="Model Catalog"
		description="Toggle individual models or test latency against Kilo Code gateway."
		models={kilocodeState.models}
		loading={kilocodeState.loading}
		onToggleModel={(model, enabled) => kilocodeState.toggleModel(model.id, enabled)}
		onToggleAll={(enabled) => kilocodeState.toggleAll(enabled)}
		onSelectChatModel={(id) => (selectedChatModel = id)}
		onAddModel={(data) => kilocodeState.addCustomModel(data)}
		onDeleteModel={(id) => kilocodeState.deleteCustomModel(id)}
	/>
</div>
