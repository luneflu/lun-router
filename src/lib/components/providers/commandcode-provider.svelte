<script lang="ts">
	import * as Card from '$lib/components/ui/card';
	import { Button } from '$lib/components/ui/button';
	import { Badge } from '$lib/components/ui/badge';
	import { CommandCodeState } from '$lib/state/commandcode.svelte';
	import { toast } from '$lib/state/toast.svelte';
	import ModelTestChat from './model-test-chat.svelte';
	import ModelListCard from './model-list-card.svelte';
	import AccountsCard from './commandcode/accounts-card.svelte';

	import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';
	import TerminalIcon from '@lucide/svelte/icons/terminal';

	interface Props {
		commandCodeState: CommandCodeState;
	}

	let { commandCodeState }: Props = $props();

	let selectedChatModel = $state('deepseek/deepseek-v4-flash');
	let syncingModels = $state(false);

	async function handleSyncModels() {
		try {
			syncingModels = true;
			const res = await commandCodeState.syncModels();
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
						class="flex size-14 shrink-0 items-center justify-center rounded-2xl border border-indigo-500/20 bg-indigo-500/10 text-indigo-500 shadow-xs"
					>
						<TerminalIcon class="size-7" />
					</div>
					<div class="flex flex-col gap-1.5">
						<div class="flex flex-wrap items-center gap-2.5">
							<h2 class="text-xl font-bold tracking-tight text-foreground">Command Code</h2>
							<Badge
								variant="outline"
								class="border-indigo-500/30 bg-indigo-500/10 font-medium text-indigo-600 dark:text-indigo-400"
							>
								CLI Protocol
							</Badge>
							{#if commandCodeState.activeAccountsCount > 0}
								<Badge
									variant="outline"
									class="border-emerald-500/30 bg-emerald-500/10 font-medium text-emerald-600 dark:text-emerald-400"
								>
									Ready ({commandCodeState.activeAccountsCount} active)
								</Badge>
							{:else}
								<Badge variant="secondary" class="font-medium text-muted-foreground">
									No Keys Connected
								</Badge>
							{/if}
						</div>
						<p class="max-w-2xl text-xs leading-relaxed text-muted-foreground">
							Talks the Command Code CLI wire protocol (<code class="font-mono"
								>POST /alpha/generate</code
							>) with CLI headers and NDJSON stream events translated to OpenAI SSE. Needs a CLI API
							key, not a Provider API key.
						</p>
					</div>
				</div>

				<div class="flex shrink-0 items-center gap-2">
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
					<span class="text-2xl font-bold tracking-tight text-foreground"
						>{commandCodeState.totalCount}</span
					>
					<span class="text-xs text-muted-foreground">available</span>
				</div>
			</div>

			<div class="flex flex-col gap-1 p-4 sm:p-5">
				<span class="text-[11px] font-medium text-muted-foreground">Enabled Models</span>
				<div class="flex items-baseline gap-2">
					<span class="text-2xl font-bold tracking-tight text-emerald-600 dark:text-emerald-400">
						{commandCodeState.enabledCount}
					</span>
					<span class="text-xs text-muted-foreground">active</span>
				</div>
			</div>

			<div class="flex flex-col gap-1 p-4 sm:p-5">
				<span class="text-[11px] font-medium text-muted-foreground">Active Keys</span>
				<div class="flex items-baseline gap-2">
					<span class="text-2xl font-bold tracking-tight text-foreground">
						{commandCodeState.activeAccountsCount}
					</span>
					<span class="text-xs text-muted-foreground"
						>/ {commandCodeState.totalAccountsCount} total</span
					>
				</div>
			</div>

			<div class="flex flex-col gap-1 p-4 sm:p-5">
				<span class="text-[11px] font-medium text-muted-foreground">Wire</span>
				<div class="flex items-baseline gap-2">
					<span class="text-base font-bold text-foreground">/alpha/generate</span>
					<span class="text-[11px] text-muted-foreground">NDJSON</span>
				</div>
			</div>
		</Card.Content>
	</Card.Root>

	<!-- Accounts Management Section -->
	<AccountsCard {commandCodeState} />

	<!-- Interactive Test Chat -->
	<ModelTestChat bind:selectedModelId={selectedChatModel} models={commandCodeState.models} />

	<!-- Models Catalog Card -->
	<ModelListCard
		title="Model Catalog"
		description="Toggle individual models or test latency against Command Code."
		models={commandCodeState.models}
		loading={commandCodeState.loading}
		onToggleModel={(model, enabled) => commandCodeState.toggleModel(model.id, enabled)}
		onToggleAll={(enabled) => commandCodeState.toggleAll(enabled)}
		onSelectChatModel={(id) => (selectedChatModel = id)}
		onAddModel={(data) => commandCodeState.addCustomModel(data)}
		onDeleteModel={(id) => commandCodeState.deleteCustomModel(id)}
	/>
</div>
