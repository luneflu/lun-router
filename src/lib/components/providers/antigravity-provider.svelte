<script lang="ts">
	import * as Card from '$lib/components/ui/card';
	import * as Table from '$lib/components/ui/table';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Badge } from '$lib/components/ui/badge';
	import { Switch } from '$lib/components/ui/switch';
	import { cn } from '$lib/utils';
	import type { ProviderModel } from '$lib/types';
	import { AntigravityState } from '$lib/state/antigravity.svelte';
	import { pingModel } from '$lib/api/providers';
	import { toast } from '$lib/state/toast.svelte';
	import ModelTestChat from './model-test-chat.svelte';
	import AccountsCard from './antigravity/accounts-card.svelte';
	import QuotaTrackerCard from './antigravity/quota-tracker-card.svelte';

	import RocketIcon from '@lucide/svelte/icons/rocket';
	import SearchIcon from '@lucide/svelte/icons/search';
	import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';
	import PlayIcon from '@lucide/svelte/icons/play';
	import SquareIcon from '@lucide/svelte/icons/square';
	import FlaskConicalIcon from '@lucide/svelte/icons/flask-conical';
	import MessageSquareIcon from '@lucide/svelte/icons/message-square';

	interface Props {
		antigravityState: AntigravityState;
	}

	let { antigravityState }: Props = $props();

	let selectedChatModel = $state('gemini-3.8-flash');
	let isTestingAll = $state(false);
	let testProgress = $state({ current: 0, total: 0 });
	let testingModelIds = $state<string[]>([]);
	let testResults = $state<Record<string, { ok: boolean; latencyMs?: number; error?: string }>>({});
	let testConcurrency = $state(4);
	let shouldCancelTestAll = false;

	let togglingId = $state<string | null>(null);
	let bulkToggling = $state(false);
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

	async function handleToggleModel(model: ProviderModel, checked: boolean) {
		try {
			togglingId = model.id;
			await antigravityState.toggleModel(model.id, checked);
		} catch (e: unknown) {
			toast.error(model.id, e instanceof Error ? e.message : 'Failed to update model');
		} finally {
			togglingId = null;
		}
	}

	async function handleToggleAll(enabled: boolean) {
		try {
			bulkToggling = true;
			await antigravityState.toggleAll(enabled);
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

	// Ping model with empty prompt probe
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
		const targetModels = antigravityState.models.filter((m) => m.enabled);
		if (targetModels.length === 0) {
			toast.info('No Models', 'No enabled models to test');
			return;
		}

		if (antigravityState.activeAccountsCount === 0) {
			toast.error('No Accounts', 'Please add at least one active Antigravity account first');
			return;
		}

		isTestingAll = true;
		shouldCancelTestAll = false;
		testProgress = { current: 0, total: targetModels.length };

		let successCount = 0;
		let failCount = 0;
		let completed = 0;
		let cursor = 0;

		toast.info(
			'Testing Started',
			`Running probe test for ${targetModels.length} models (${testConcurrency}x parallel)...`
		);

		const workerCount = Math.min(testConcurrency, targetModels.length);
		const workers = Array.from({ length: workerCount }, async () => {
			while (cursor < targetModels.length && !shouldCancelTestAll) {
				const modelIndex = cursor++;
				const model = targetModels[modelIndex];
				const ok = await handleTestModel(model.id, true);
				if (ok) successCount++;
				else failCount++;
				completed++;
				testProgress = { current: completed, total: targetModels.length };
			}
		});

		await Promise.all(workers);

		isTestingAll = false;
		testingModelIds = [];

		if (!shouldCancelTestAll) {
			toast.info(
				'Test Completed',
				`${successCount} passed, ${failCount} failed (${testConcurrency}x parallel)`
			);
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
						disabled={syncingModels || isTestingAll}
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
						disabled={antigravityState.loading || isTestingAll}
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
	<Card.Root class="overflow-hidden p-0">
		<Card.Header
			class="flex flex-col gap-4 border-b border-border p-4 sm:flex-row sm:items-center sm:justify-between sm:p-5"
		>
			<div class="flex flex-col gap-1">
				<div class="flex items-center gap-2">
					<Card.Title class="text-base font-semibold">Available Models</Card.Title>
					<Badge variant="outline" class="font-mono text-xs">
						{antigravityState.filtered.length} of {antigravityState.totalCount}
					</Badge>
				</div>
				<Card.Description class="text-xs">
					Toggle Antigravity models on or off. Test models using probe ping.
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
						bind:value={antigravityState.searchQuery}
						placeholder="Search models..."
						class="h-8 ps-8 text-xs"
					/>
				</div>

				<!-- Batch concurrency & Test All button -->
				<div class="flex items-center gap-1.5">
					<select
						bind:value={testConcurrency}
						disabled={isTestingAll}
						class="h-8 rounded-md border border-input bg-background px-2 py-1 text-xs shadow-xs focus-visible:ring-1 focus-visible:ring-ring focus-visible:outline-none"
						title="Batch test concurrency (parallel pings)"
					>
						<option value={1}>1x (Serial)</option>
						<option value={2}>2x</option>
						<option value={4}>4x (Batch)</option>
						<option value={8}>8x (Turbo)</option>
					</select>

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
							disabled={antigravityState.loading || antigravityState.enabledCount === 0}
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
						disabled={bulkToggling || antigravityState.loading || isTestingAll}
						onclick={() => handleToggleAll(true)}
						title="Enable all models"
					>
						Enable All
					</Button>
					<Button
						variant="outline"
						size="sm"
						disabled={bulkToggling || antigravityState.loading || isTestingAll}
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
					antigravityState.filter === 'all'
						? 'bg-background text-foreground shadow-xs'
						: 'text-muted-foreground hover:text-foreground'
				)}
				onclick={() => (antigravityState.filter = 'all')}
			>
				All ({antigravityState.totalCount})
			</button>
			<button
				type="button"
				class={cn(
					'rounded-md px-2.5 py-1 text-xs font-medium transition-colors',
					antigravityState.filter === 'enabled'
						? 'bg-background text-foreground shadow-xs'
						: 'text-muted-foreground hover:text-foreground'
				)}
				onclick={() => (antigravityState.filter = 'enabled')}
			>
				Enabled ({antigravityState.enabledCount})
			</button>
			<button
				type="button"
				class={cn(
					'rounded-md px-2.5 py-1 text-xs font-medium transition-colors',
					antigravityState.filter === 'disabled'
						? 'bg-background text-foreground shadow-xs'
						: 'text-muted-foreground hover:text-foreground'
				)}
				onclick={() => (antigravityState.filter = 'disabled')}
			>
				Disabled ({antigravityState.disabledCount})
			</button>
		</div>

		<!-- Models Table -->
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
					{#if antigravityState.loading}
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
					{:else if antigravityState.filtered.length === 0}
						<Table.Row>
							<Table.Cell colspan={3} class="h-32 text-center text-muted-foreground">
								<div class="flex flex-col items-center justify-center gap-1">
									<p class="text-xs font-medium text-foreground">No models found</p>
									<p class="text-xs">No models match the filter criteria.</p>
								</div>
							</Table.Cell>
						</Table.Row>
					{:else}
						{#each antigravityState.filtered as m (m.id)}
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
											class="h-7 px-2 text-xs text-amber-500 hover:bg-amber-500/10"
											disabled={testingModelIds.includes(m.id)}
											onclick={() => handleTestModel(m.id)}
											title={`Send empty prompt ping to ${m.id}`}
										>
											{#if testingModelIds.includes(m.id)}
												<RefreshCwIcon data-icon="inline-start" class="size-3 animate-spin" />
												Ping
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
