<script lang="ts">
	import * as Card from '$lib/components/ui/card';
	import { Button } from '$lib/components/ui/button';
	import { Badge } from '$lib/components/ui/badge';
	import { cn } from '$lib/utils';
	import { AntigravityState } from '$lib/state/antigravity.svelte';
	import { toast } from '$lib/state/toast.svelte';
	import ModelTestChat from './model-test-chat.svelte';
	import ModelListCard from './model-list-card.svelte';
	import AccountsCard from './antigravity/accounts-card.svelte';
	import QuotaTrackerCard from './antigravity/quota-tracker-card.svelte';

	import RocketIcon from '@lucide/svelte/icons/rocket';
	import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';

	interface Props {
		antigravityState: AntigravityState;
	}

	let { antigravityState }: Props = $props();

	let selectedChatModel = $state('gemini-3.8-flash');
	let syncingModels = $state(false);

	async function handleSyncModels() {
		try {
			syncingModels = true;
			const res = await antigravityState.syncModels();
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
	<!-- Top Provider Status Header Card -->
	<Card.Root>
		<Card.Header class="p-5 sm:p-6">
			<div class="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between sm:gap-6">
				<div class="flex items-center gap-3.5">
					<div
						class="flex size-11 items-center justify-center rounded-xl border border-amber-500/20 bg-amber-500/10 text-amber-500 shadow-xs"
					>
						<RocketIcon class="size-6" />
					</div>
					<div class="flex flex-col gap-0.5">
						<div class="flex items-center gap-2">
							<h1 class="text-xl font-bold tracking-tight text-foreground">Google Antigravity</h1>
							<Badge
								variant="outline"
								class="border-amber-500/30 bg-amber-500/10 font-mono text-xs text-amber-500"
							>
								v{antigravityState.version}
							</Badge>
							<Badge
								variant="secondary"
								class="h-5 px-1.5 text-[10px] font-medium text-emerald-500"
							>
								9router
							</Badge>
						</div>
						<Card.Description class="text-xs text-muted-foreground">
							{antigravityState.activeAccountsCount} active accounts • {antigravityState.enabledCount}
							enabled / {antigravityState.totalCount} total models
						</Card.Description>
					</div>
				</div>

				<div class="flex items-center gap-2 self-start sm:self-auto">
					<Button
						variant="outline"
						size="sm"
						disabled={syncingModels}
						onclick={handleSyncModels}
						title="Sync latest model catalog from Google Cloud Code"
					>
						<RefreshCwIcon
							data-icon="inline-start"
							class={cn('size-3.5', syncingModels && 'animate-spin')}
						/>
						Sync Models
					</Button>
					<Button
						variant="outline"
						size="sm"
						disabled={antigravityState.loading}
						onclick={() => antigravityState.load()}
					>
						<RefreshCwIcon
							data-icon="inline-start"
							class={cn('size-3.5', antigravityState.loading && 'animate-spin')}
						/>
						Refresh Provider
					</Button>
				</div>
			</div>
		</Card.Header>
	</Card.Root>

	<!-- Google Accounts Card -->
	<AccountsCard {antigravityState} />

	<!-- Quota Tracker Card -->
	<QuotaTrackerCard {antigravityState} />

	<!-- Temporary Model Testing Chat (Reused Component) -->
	<ModelTestChat models={antigravityState.models} bind:selectedModelId={selectedChatModel} />

	<!-- Models Management Card -->
	<ModelListCard
		title="Available Models"
		description="Toggle Antigravity models on or off. Test models using probe ping."
		models={antigravityState.models}
		loading={antigravityState.loading}
		onToggleModel={(model, enabled) => antigravityState.toggleModel(model.id, enabled)}
		onToggleAll={(enabled) => antigravityState.toggleAll(enabled)}
		onSelectChatModel={(id) => (selectedChatModel = id)}
	/>
</div>
