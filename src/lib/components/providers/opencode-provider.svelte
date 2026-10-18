<script lang="ts">
	import { onMount } from 'svelte';
	import * as Card from '$lib/components/ui/card';
	import { Button } from '$lib/components/ui/button';
	import { Badge } from '$lib/components/ui/badge';
	import { Switch } from '$lib/components/ui/switch';
	import { cn } from '$lib/utils';
	import { ProviderState } from '$lib/state/provider.svelte';
	import { toast } from '$lib/state/toast.svelte';
	import ModelTestChat from './model-test-chat.svelte';
	import ModelListCard from './model-list-card.svelte';

	import BotIcon from '@lucide/svelte/icons/bot';
	import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';
	import GlobeIcon from '@lucide/svelte/icons/globe';
	import ShieldCheckIcon from '@lucide/svelte/icons/shield-check';
	import {
		fetchOpenCodeProxyStatus,
		updateOpenCodeProxy,
		refreshOpenCodeProxy,
		type OpenCodeProxyStatus
	} from '$lib/api/providers';

	interface Props {
		providerState: ProviderState;
	}

	let { providerState }: Props = $props();

	let selectedChatModel = $state('big-pickle');
	let checkingVersion = $state(false);
	let syncingModels = $state(false);

	let proxyStatus = $state<OpenCodeProxyStatus | null>(null);
	let refreshingProxy = $state(false);
	let togglingProxy = $state(false);

	async function loadProxyStatus() {
		try {
			proxyStatus = await fetchOpenCodeProxyStatus();
		} catch {}
	}

	async function handleToggleProxy(checked: boolean) {
		try {
			togglingProxy = true;
			proxyStatus = await updateOpenCodeProxy(checked);
			toast.success('Proxy Updated', checked ? 'Proxy pool active' : 'Proxy pool standby (auto on 429)');
		} catch (err: any) {
			toast.error('Proxy Error', err.message);
		} finally {
			togglingProxy = false;
		}
	}

	async function handleRefreshProxy() {
		try {
			refreshingProxy = true;
			const res = await refreshOpenCodeProxy();
			if (res.ok && res.status) {
				proxyStatus = res.status;
				toast.success(
					'Proxies Updated',
					`Fetched ${res.status.total_count} proxies (DE: ${res.status.germany_count}, EU: ${res.status.europe_count})`
				);
			} else {
				toast.error('Proxy Refresh Failed', res.error || 'Failed');
			}
		} finally {
			refreshingProxy = false;
		}
	}

	onMount(() => {
		loadProxyStatus();
	});

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
							<Card.Title class="text-lg font-bold">OpenCode Free</Card.Title>
							<Badge variant="outline" class="font-mono text-xs">v{providerState.version}</Badge>
						</div>
						<Card.Description class="text-xs text-muted-foreground">
							{providerState.enabledCount} enabled / {providerState.totalCount} total models • dynamic
							upstream sync
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

	<!-- Geonode Proxy Status Cards -->
	<div class="grid grid-cols-1 gap-4 md:grid-cols-3">
		<Card.Root>
			<Card.Header class="pb-2">
				<Card.Title class="flex items-center gap-2 text-sm font-medium text-muted-foreground">
					<GlobeIcon class="size-4" />
					Geonode Proxy Pool
				</Card.Title>
			</Card.Header>
			<Card.Content>
				<div class="flex items-baseline justify-between">
					<span class="text-xl font-bold">
						{proxyStatus?.total_count ?? 0} Proxies
					</span>
					<Badge variant="outline" class="text-xs">
						DE: {proxyStatus?.germany_count ?? 0} • EU: {proxyStatus?.europe_count ?? 0}
					</Badge>
				</div>
				<p class="mt-1 truncate text-xs text-muted-foreground">
					Priority: Germany first, Europe second. Sorted by uptime & speed.
				</p>
			</Card.Content>
		</Card.Root>

		<Card.Root>
			<Card.Header class="pb-2 flex flex-row items-center justify-between space-y-0">
				<Card.Title class="flex items-center gap-2 text-sm font-medium text-muted-foreground">
					<ShieldCheckIcon class="size-4" />
					Free Limit Failover
				</Card.Title>
				<Switch
					checked={proxyStatus?.enabled ?? false}
					disabled={togglingProxy}
					onCheckedChange={handleToggleProxy}
				/>
			</Card.Header>
			<Card.Content>
				<div class="flex items-baseline justify-between">
					<span class="text-xl font-bold">
						{proxyStatus?.enabled ? 'Always Active' : 'Auto on 429'}
					</span>
					<Badge variant={proxyStatus?.enabled ? 'default' : 'secondary'} class="text-xs">
						{proxyStatus?.enabled ? 'Enforced' : 'Standby'}
					</Badge>
				</div>
				<p class="mt-1 text-xs text-muted-foreground">
					{proxyStatus?.enabled
						? 'Routing all requests via proxy.'
						: 'Auto-routes through proxy when free limit error occurs.'}
				</p>
			</Card.Content>
		</Card.Root>

		<Card.Root>
			<Card.Header class="pb-2 flex flex-row items-center justify-between space-y-0">
				<Card.Title class="flex items-center gap-2 text-sm font-medium text-muted-foreground">
					Active Routing
				</Card.Title>
				<Button
					variant="ghost"
					size="sm"
					class="h-6 px-2 text-xs"
					disabled={refreshingProxy}
					onclick={handleRefreshProxy}
				>
					<RefreshCwIcon class={cn('size-3', refreshingProxy && 'animate-spin')} />
					<span class="ml-1">Sync</span>
				</Button>
			</Card.Header>
			<Card.Content>
				<div class="flex items-baseline justify-between">
					<span class="text-sm font-mono font-semibold truncate">
						{proxyStatus?.active_proxy
							? `${proxyStatus.active_proxy.ip}:${proxyStatus.active_proxy.port} (${proxyStatus.active_proxy.country})`
							: 'Direct (No Proxy)'}
					</span>
				</div>
				<p class="mt-1 text-xs text-muted-foreground">
					{proxyStatus?.failed_count
						? `${proxyStatus.failed_count} dead/rate-limited in cooldown`
						: 'All healthy'}
				</p>
			</Card.Content>
		</Card.Root>
	</div>

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
	/>
</div>
