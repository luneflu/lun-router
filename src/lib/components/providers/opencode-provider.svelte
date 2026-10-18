<script lang="ts">
	import { onMount } from 'svelte';
	import * as Card from '$lib/components/ui/card';
	import * as Table from '$lib/components/ui/table';
	import * as Select from '$lib/components/ui/select';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Badge } from '$lib/components/ui/badge';
	import { Switch } from '$lib/components/ui/switch';
	import { cn } from '$lib/utils';
	import type { ProviderModel } from '$lib/types';
	import { ProviderState } from '$lib/state/provider.svelte';
	import { pingModel } from '$lib/api/providers';
	import { toast } from '$lib/state/toast.svelte';
	import ModelTestChat from './model-test-chat.svelte';

	import BotIcon from '@lucide/svelte/icons/bot';
	import SearchIcon from '@lucide/svelte/icons/search';
	import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';
	import PlayIcon from '@lucide/svelte/icons/play';
	import SquareIcon from '@lucide/svelte/icons/square';
	import FlaskConicalIcon from '@lucide/svelte/icons/flask-conical';
	import MessageSquareIcon from '@lucide/svelte/icons/message-square';
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
	let isTestingAll = $state(false);
	let testProgress = $state({ current: 0, total: 0 });
	let testingModelIds = $state<string[]>([]);
	let testResults = $state<Record<string, { ok: boolean; latencyMs?: number; error?: string }>>({});
	let testConcurrency = $state(4);
	let shouldCancelTestAll = false;

	let togglingId = $state<string | null>(null);
	let bulkToggling = $state(false);
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

	async function handleToggleModel(model: ProviderModel, checked: boolean) {
		try {
			togglingId = model.id;
			await providerState.toggleModel(model.id, checked);
		} catch (e: unknown) {
			toast.error(model.id, e instanceof Error ? e.message : 'Failed to update model');
		} finally {
			togglingId = null;
		}
	}

	async function handleToggleAll(enabled: boolean) {
		try {
			bulkToggling = true;
			await providerState.toggleAll(enabled);
			toast.info('Models Updated', `${enabled ? 'Enabled' : 'Disabled'} all models`);
		} catch (e: unknown) {
			toast.error('Bulk Update Failed', e instanceof Error ? e.message : 'Error updating models');
		} finally {
			bulkToggling = false;
		}
	}

	function handleSelectChatModel(modelId: string) {
		selectedChatModel = modelId;
		window.scrollTo({ top: 0, behavior: 'smooth' });
	}

	// Ping model with empty prompt like 9router
	async function handleTestModel(modelId: string, silent = false): Promise<boolean> {
		if (!testingModelIds.includes(modelId)) {
			testingModelIds = [...testingModelIds, modelId];
		}

		try {
			const res = await pingModel(modelId, '');
			testResults = {
				...testResults,
				[modelId]: { ok: res.ok, latencyMs: res.latency_ms, error: res.error }
			};
			if (res.ok) {
				if (!silent) {
					toast.success(modelId, `Reachable (${res.latency_ms}ms)`);
				}
				return true;
			} else {
				if (!silent) {
					toast.error(modelId, res.error || 'Test failed');
				}
				return false;
			}
		} catch (e: unknown) {
			const errMsg = e instanceof Error ? e.message : 'Network error';
			testResults = {
				...testResults,
				[modelId]: { ok: false, error: errMsg }
			};
			if (!silent) {
				toast.error(modelId, errMsg);
			}
			return false;
		} finally {
			testingModelIds = testingModelIds.filter((id) => id !== modelId);
		}
	}

	// Test all enabled models concurrently in batch
	async function handleTestAll() {
		const targetModels = providerState.models.filter((m) => m.enabled);
		if (targetModels.length === 0) {
			toast.info('No Models', 'No enabled models to test');
			return;
		}

		isTestingAll = true;
		shouldCancelTestAll = false;
		testProgress = { current: 0, total: targetModels.length };

		let passed = 0;
		let failed = 0;
		let completed = 0;
		let cursor = 0;

		const workerCount = Math.min(testConcurrency, targetModels.length);
		const workers = Array.from({ length: workerCount }, async () => {
			while (cursor < targetModels.length && !shouldCancelTestAll) {
				const modelIndex = cursor++;
				const m = targetModels[modelIndex];
				const ok = await handleTestModel(m.id, true);
				if (ok) passed++;
				else failed++;
				completed++;
				testProgress = { current: completed, total: targetModels.length };
			}
		});

		await Promise.all(workers);

		isTestingAll = false;
		testingModelIds = [];

		if (!shouldCancelTestAll) {
			if (failed === 0) {
				toast.success(
					'Batch Test Completed',
					`All ${passed} models reachable (${testConcurrency}x parallel)`
				);
			} else {
				toast.info(
					'Batch Test Completed',
					`${passed} passed, ${failed} failed (${testConcurrency}x parallel)`
				);
			}
		}
	}

	function handleStopTestAll() {
		shouldCancelTestAll = true;
		isTestingAll = false;
		testingModelIds = [];
		toast.info('Stopped', 'Test all canceled');
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
						disabled={syncingModels || isTestingAll}
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
						disabled={checkingVersion || isTestingAll}
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
						disabled={providerState.loading || isTestingAll}
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
	<Card.Root class="overflow-hidden p-0">
		<Card.Header
			class="flex flex-col gap-4 border-b border-border p-4 sm:flex-row sm:items-center sm:justify-between sm:p-5"
		>
			<div class="flex flex-col gap-1">
				<div class="flex items-center gap-2">
					<Card.Title class="text-base font-semibold">Available Models</Card.Title>
					<Badge variant="outline" class="font-mono text-xs">
						{providerState.filtered.length} of {providerState.totalCount}
					</Badge>
				</div>
				<Card.Description class="text-xs">
					Toggle models on or off. Test models using empty prompt probe.
				</Card.Description>
			</div>

			<div class="flex flex-wrap items-center gap-2">
				<!-- Search -->
				<div class="relative w-full sm:w-52">
					<SearchIcon
						class="pointer-events-none absolute start-2.5 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground"
					/>
					<Input
						type="search"
						bind:value={providerState.searchQuery}
						placeholder="Search models..."
						class="h-8 ps-8 text-xs"
					/>
				</div>

				<!-- Batch concurrency & Test All button -->
				<div class="flex items-center gap-1.5">
					<Select.Root
						type="single"
						value={String(testConcurrency)}
						onValueChange={(v) => {
							if (v) testConcurrency = Number(v);
						}}
						disabled={isTestingAll}
					>
						<Select.Trigger
							class="h-8 w-28 text-xs"
							title="Batch test concurrency (parallel pings)"
						>
							<Select.Value placeholder="Concurrency" />
						</Select.Trigger>
						<Select.Content>
							<Select.Group>
								<Select.Item value="1" label="1x (Serial)">1x (Serial)</Select.Item>
								<Select.Item value="2" label="2x">2x</Select.Item>
								<Select.Item value="4" label="4x (Batch)">4x (Batch)</Select.Item>
								<Select.Item value="8" label="8x (Turbo)">8x (Turbo)</Select.Item>
							</Select.Group>
						</Select.Content>
					</Select.Root>

					{#if isTestingAll}
						<div class="flex items-center gap-1.5">
							<Button variant="default" size="sm" disabled class="gap-1.5 text-xs">
								<RefreshCwIcon data-icon="inline-start" class="size-3.5 animate-spin" />
								Testing ({testProgress.current}/{testProgress.total})
							</Button>
							<Button
								variant="destructive"
								size="sm"
								onclick={handleStopTestAll}
								title="Stop testing"
							>
								<SquareIcon class="size-3" />
							</Button>
						</div>
					{:else}
						<Button
							variant="default"
							size="sm"
							disabled={providerState.loading || providerState.enabledCount === 0}
							onclick={handleTestAll}
							title="Test all enabled models in batch"
						>
							<FlaskConicalIcon data-icon="inline-start" class="size-3.5" />
							Test All ({testConcurrency}x)
						</Button>
					{/if}
				</div>

				<!-- Bulk enable/disable buttons -->
				<div class="flex items-center gap-1">
					<Button
						variant="outline"
						size="sm"
						disabled={bulkToggling || providerState.loading || isTestingAll}
						onclick={() => handleToggleAll(true)}
						title="Enable all models"
					>
						Enable All
					</Button>
					<Button
						variant="outline"
						size="sm"
						disabled={bulkToggling || providerState.loading || isTestingAll}
						onclick={() => handleToggleAll(false)}
						title="Disable all models"
					>
						Disable All
					</Button>
				</div>
			</div>
		</Card.Header>

		<!-- Filter Tabs Bar -->
		<div class="flex items-center gap-1 border-b border-border bg-muted/30 px-4 py-2">
			<button
				type="button"
				class={cn(
					'rounded-md px-2.5 py-1 text-xs font-medium transition-colors',
					providerState.filter === 'all'
						? 'bg-background text-foreground shadow-xs'
						: 'text-muted-foreground hover:text-foreground'
				)}
				onclick={() => (providerState.filter = 'all')}
			>
				All ({providerState.totalCount})
			</button>
			<button
				type="button"
				class={cn(
					'rounded-md px-2.5 py-1 text-xs font-medium transition-colors',
					providerState.filter === 'enabled'
						? 'bg-background text-foreground shadow-xs'
						: 'text-muted-foreground hover:text-foreground'
				)}
				onclick={() => (providerState.filter = 'enabled')}
			>
				Enabled ({providerState.enabledCount})
			</button>
			<button
				type="button"
				class={cn(
					'rounded-md px-2.5 py-1 text-xs font-medium transition-colors',
					providerState.filter === 'disabled'
						? 'bg-background text-foreground shadow-xs'
						: 'text-muted-foreground hover:text-foreground'
				)}
				onclick={() => (providerState.filter = 'disabled')}
			>
				Disabled ({providerState.disabledCount})
			</button>
		</div>

		<!-- Models Table: Only 3 clean columns -->
		<Card.Content class="overflow-x-auto p-0">
			<Table.Root>
				<Table.Header>
					<Table.Row class="hover:bg-transparent">
						<Table.Head>MODEL ID</Table.Head>
						<Table.Head class="w-[120px]">ENABLED</Table.Head>
						<Table.Head class="w-[100px] text-end">ACTIONS</Table.Head>
					</Table.Row>
				</Table.Header>
				<Table.Body>
					{#if providerState.loading}
						<Table.Row>
							<Table.Cell colspan={3} class="h-32 text-center text-muted-foreground">
								<div class="flex flex-col items-center justify-center gap-2">
									<div
										class="size-4 animate-spin rounded-full border-2 border-primary border-t-transparent"
									></div>
									<span class="text-xs">Loading models...</span>
								</div>
							</Table.Cell>
						</Table.Row>
					{:else if providerState.filtered.length === 0}
						<Table.Row>
							<Table.Cell colspan={3} class="h-32 text-center text-muted-foreground">
								<div class="flex flex-col items-center justify-center gap-1">
									<p class="text-xs font-medium text-foreground">No models found</p>
									<p class="text-xs">No models match the filter criteria.</p>
								</div>
							</Table.Cell>
						</Table.Row>
					{:else}
						{#each providerState.filtered as m (m.id)}
							<Table.Row class={cn(!m.enabled && 'bg-muted/10 opacity-60')}>
								<Table.Cell class="font-mono text-xs">
									<div class="flex flex-col gap-1">
										<div class="flex flex-wrap items-center gap-1.5">
											<span class="font-semibold text-foreground">{m.id}</span>
											{#if m.thinking_levels && m.thinking_levels.length > 0}
												<Badge
													variant="outline"
													class="border-amber-500/30 bg-amber-500/10 px-1.5 py-0 text-[10px] font-normal text-amber-600 dark:text-amber-400"
												>
													Thinking ({m.thinking_levels.filter((l) => l !== 'none').join(', ')})
												</Badge>
											{/if}
											{#if m.capabilities?.vision}
												<Badge
													variant="outline"
													class="border-sky-500/30 bg-sky-500/10 px-1.5 py-0 text-[10px] font-normal text-sky-600 dark:text-sky-400"
												>
													Vision
												</Badge>
											{/if}
										</div>
										{#if m.context_length}
											<span class="text-[10px] text-muted-foreground">
												Context: {Math.round(m.context_length / 1000)}k | Max output: {Math.round(
													(m.max_completion_tokens || 64000) / 1000
												)}k
											</span>
										{/if}
									</div>
								</Table.Cell>

								<Table.Cell>
									<div class="flex items-center gap-2">
										<Switch
											checked={m.enabled}
											disabled={togglingId === m.id || bulkToggling || isTestingAll}
											onCheckedChange={(checked) => handleToggleModel(m, checked)}
											size="sm"
											aria-label={`Toggle model ${m.id}`}
										/>
										<span class="text-[11px] text-muted-foreground">
											{m.enabled ? 'On' : 'Off'}
										</span>
									</div>
								</Table.Cell>

								<Table.Cell class="text-end">
									<div class="flex items-center justify-end gap-1.5">
										{#if testResults[m.id]}
											{#if testResults[m.id].ok}
												<Badge
													variant="outline"
													class="border-green-500/30 bg-green-500/10 px-1.5 py-0 font-mono text-[10px] text-green-600 dark:text-green-400"
												>
													{testResults[m.id].latencyMs}ms
												</Badge>
											{:else}
												<Badge
													variant="outline"
													class="border-destructive/30 bg-destructive/10 px-1.5 py-0 font-mono text-[10px] text-destructive"
													title={testResults[m.id].error || 'Failed'}
												>
													Fail
												</Badge>
											{/if}
										{/if}
										<Button
											variant="ghost"
											size="sm"
											class="h-7 px-2 text-xs text-muted-foreground hover:bg-muted hover:text-foreground"
											onclick={() => handleSelectChatModel(m.id)}
											title={`Select ${m.id} in temporary chat above`}
										>
											<MessageSquareIcon data-icon="inline-start" class="size-3" />
											Chat
										</Button>
										<Button
											variant="ghost"
											size="sm"
											class="h-7 px-2 text-xs text-primary hover:bg-primary/10"
											disabled={testingModelIds.includes(m.id)}
											onclick={() => handleTestModel(m.id)}
											title={`Send empty prompt ping to ${m.id}`}
										>
											{#if testingModelIds.includes(m.id)}
												<RefreshCwIcon data-icon="inline-start" class="size-3 animate-spin" />
												Testing
											{:else}
												<PlayIcon data-icon="inline-start" class="size-3" />
												Ping
											{/if}
										</Button>
									</div>
								</Table.Cell>
							</Table.Row>
						{/each}
					{/if}
				</Table.Body>
			</Table.Root>
		</Card.Content>
	</Card.Root>
</div>
