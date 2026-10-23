<script lang="ts">
	import * as Card from '$lib/components/ui/card';
	import { Button } from '$lib/components/ui/button';
	import { Badge } from '$lib/components/ui/badge';
	import { cn } from '$lib/utils';
	import { ProviderState } from '$lib/state/provider.svelte';
	import { toast } from '$lib/state/toast.svelte';
	import ModelTestChat from './model-test-chat.svelte';
	import ModelListCard from './model-list-card.svelte';
	import AccountsCard from './opencode/accounts-card.svelte';
	import QuotaTrackerCard from './quota-tracker-card.svelte';

	import BotIcon from '@lucide/svelte/icons/bot';
	import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';

	interface Props {
		providerState: ProviderState;
	}

	let { providerState }: Props = $props();

	let selectedChatModel = $state('big-pickle');
	let checkingVersion = $state(false);
	let syncingModels = $state(false);

	async function handleSyncModels() {
		try {
			syncingModels = true;
			const res = await providerState.syncModels();
			if (res.ok) {
				toast.success('Models Synced', `Up to date: ${res.count} models available`);
			} else {
				toast.error('Sync Failed', res.error || 'Failed to sync models');
			}
		} finally {
			syncingModels = false;
		}
	}

	async function handleCheckVersion() {
		try {
			checkingVersion = true;
			const res = await providerState.checkVersionUpdate();
			if (res.ok) {
				if (res.updated) {
					toast.success('Version Updated', `Detected new version v${res.version}`);
				} else {
					toast.info('Up to Date', `Already using latest version v${res.version}`);
				}
			} else {
				toast.error('Version Check Failed', res.error || 'Could not reach GitHub');
			}
		} finally {
			checkingVersion = false;
		}
	}
</script>

<div class="flex flex-col gap-6">
	<!-- Clean Header Card (No noisy endpoint or badges) -->
	<Card.Root class="overflow-hidden border-border bg-card">
		<Card.Header class="p-5">
			<div class="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
				<div class="flex items-center gap-3">
					<div
						class="flex size-10 shrink-0 items-center justify-center rounded-xl border border-primary/20 bg-primary/10 text-primary"
					>
						<BotIcon class="size-5" />
					</div>
					<div class="flex flex-col">
						<div class="flex items-center gap-2">
							<Card.Title class="text-lg font-bold">OpenCode</Card.Title>
							<Badge variant="outline" class="font-mono text-xs">v{providerState.version}</Badge>
						</div>
						<Card.Description class="text-xs text-muted-foreground">
							{providerState.enabledCount} enabled / {providerState.totalCount} total models •
							{providerState.activeAccountsCount > 0
								? `${providerState.activeAccountsCount} OpenCode account(s) active`
								: 'anonymous free tier'}
						</Card.Description>
					</div>
				</div>

				<div class="flex items-center gap-2 self-start sm:self-auto">
					<Button
						variant="outline"
						size="sm"
						disabled={syncingModels}
						onclick={handleSyncModels}
						title="Sync latest model catalog from OpenCode upstream"
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
						disabled={checkingVersion}
						onclick={handleCheckVersion}
						title="Check GitHub for OpenCode version update"
					>
						<RefreshCwIcon
							data-icon="inline-start"
							class={cn('size-3.5', checkingVersion && 'animate-spin')}
						/>
						Check Version
					</Button>
					<Button
						variant="outline"
						size="sm"
						disabled={providerState.loading}
						onclick={() => providerState.load()}
					>
						<RefreshCwIcon
							data-icon="inline-start"
							class={cn('size-3.5', providerState.loading && 'animate-spin')}
						/>
						Refresh Models
					</Button>
				</div>
			</div>
		</Card.Header>
	</Card.Root>

	<!-- OpenCode Accounts -->
	<AccountsCard {providerState} />

	<!-- OpenCode Go Quota Tracker -->
	<QuotaTrackerCard
		accounts={providerState.accounts}
		usages={providerState.usages}
		loading={providerState.quotaLoading}
		summaryKeys={['rolling', 'weekly', 'monthly']}
		onRefreshAll={() => providerState.loadQuota(undefined, true)}
		onRefreshAccount={(id) => providerState.loadQuota(id, true)}
		title="Quota Tracker"
		description="Rolling, weekly and monthly OpenCode Go limits tracked via the usage API"
		emptyTitle="No OpenCode Account"
		emptyText="Connect an OpenCode Console account above to view and track your OpenCode Go quota."
		loadingText="Loading OpenCode Go quota..."
		noMetricsText="No quota metrics recorded yet. Click Refresh Quotas to query OpenCode."
		defaultPlan="OpenCode Go"
		accent="orange"
		subtitleFor={(account) => account.orgName || 'OpenCode Go'}
		refreshAllSuccess="Quotas Refreshed"
		refreshAccountSuccess="Quota Updated"
	/>

	<!-- Temporary Model Testing Chat -->
	<ModelTestChat models={providerState.models} bind:selectedModelId={selectedChatModel} />

	<!-- Models Management Card -->
	<ModelListCard
		title="Available Models"
		description="Toggle models on or off. Test models using empty prompt probe."
		models={providerState.models}
		loading={providerState.loading}
		onToggleModel={(model, enabled) => providerState.toggleModel(model.id, enabled)}
		onToggleAll={(enabled) => providerState.toggleAll(enabled)}
		onSelectChatModel={(id) => (selectedChatModel = id)}
		onAddModel={(data) => providerState.addCustomModel(data)}
		onDeleteModel={(id) => providerState.deleteCustomModel(id)}
	/>
</div>
