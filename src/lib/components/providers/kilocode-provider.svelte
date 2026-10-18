<script lang="ts">
	import * as Card from '$lib/components/ui/card';
	import * as Table from '$lib/components/ui/table';
	import * as Select from '$lib/components/ui/select';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Badge } from '$lib/components/ui/badge';
	import { Switch } from '$lib/components/ui/switch';
	import { cn } from '$lib/utils';
	import type { ProviderModel } from '$lib/types';
	import { KilocodeState } from '$lib/state/kilocode.svelte';
	import { pingModel } from '$lib/api/providers';
	import { toast } from '$lib/state/toast.svelte';
	import ModelTestChat from './model-test-chat.svelte';
	import AccountsCard from './kilocode/accounts-card.svelte';

	import CodeIcon from '@lucide/svelte/icons/code';
	import SearchIcon from '@lucide/svelte/icons/search';
	import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';
	import PlayIcon from '@lucide/svelte/icons/play';
	import SquareIcon from '@lucide/svelte/icons/square';
	import FlaskConicalIcon from '@lucide/svelte/icons/flask-conical';
	import MessageSquareIcon from '@lucide/svelte/icons/message-square';

	interface Props {
		kilocodeState: KilocodeState;
	}

	let { kilocodeState }: Props = $props();

	let selectedChatModel = $state('anthropic/claude-sonnet-4-20250514');
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

	async function handleToggleModel(model: ProviderModel, checked: boolean) {
		try {
			togglingId = model.id;
			await kilocodeState.toggleModel(model.id, checked);
		} catch (e: unknown) {
			toast.error(model.id, e instanceof Error ? e.message : 'Failed to update model');
		} finally {
			togglingId = null;
		}
	}

	async function handleToggleAll(enabled: boolean) {
		try {
			bulkToggling = true;
			await kilocodeState.toggleAll(enabled);
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

	async function handleTestAll() {
		if (isTestingAll) return;
		shouldCancelTestAll = false;
		isTestingAll = true;

		const enabledModels = kilocodeState.models.filter((m) => m.enabled);
		testProgress = { current: 0, total: enabledModels.length };

		let passed = 0;
		let failed = 0;

		const queue = [...enabledModels];
		const runWorker = async () => {
			while (queue.length > 0 && !shouldCancelTestAll) {
				const model = queue.shift();
				if (!model) break;
				const ok = await handleTestModel(model.id, true);
				if (ok) passed++;
				else failed++;
				testProgress = { ...testProgress, current: testProgress.current + 1 };
			}
		};

		const workers = Array.from({ length: Math.min(testConcurrency, queue.length) }, () => runWorker());
		await Promise.all(workers);

		isTestingAll = false;
		if (shouldCancelTestAll) {
			toast.info('Testing Cancelled', `Tested ${testProgress.current} models`);
		} else {
			toast.info(
				'Testing Complete',
				`Results: ${passed} passed, ${failed} failed out of ${testProgress.total}`
			);
		}
	}

	function handleCancelTestAll() {
		shouldCancelTestAll = true;
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
	<Card.Root class="overflow-hidden p-0">
		<Card.Header
			class="flex flex-col gap-4 border-b border-border p-4 sm:flex-row sm:items-center sm:justify-between sm:p-5"
		>
			<div class="flex flex-col gap-1">
				<div class="flex items-center gap-2">
					<Card.Title class="text-base font-semibold">Model Catalog</Card.Title>
					<Badge variant="outline" class="font-mono text-xs">
						{kilocodeState.enabledCount} / {kilocodeState.totalCount} enabled
					</Badge>
				</div>
				<Card.Description class="text-xs">
					Toggle individual models or test latency against Kilo Code gateway.
				</Card.Description>
			</div>

			<div class="flex flex-wrap items-center gap-2">
				<!-- Search -->
				<div class="relative w-full sm:w-64">
					<SearchIcon class="absolute start-2.5 top-2.5 size-3.5 text-muted-foreground" />
					<Input
						type="search"
						placeholder="Search models..."
						class="h-8 ps-8 text-xs"
						bind:value={kilocodeState.searchQuery}
					/>
				</div>

				<!-- Bulk toggles -->
				<div class="flex items-center gap-1.5">
					<Button
						variant="outline"
						size="sm"
						class="h-8 text-xs"
						onclick={() => handleToggleAll(true)}
						disabled={bulkToggling || kilocodeState.enabledCount === kilocodeState.totalCount}
					>
						Enable All
					</Button>
					<Button
						variant="outline"
						size="sm"
						class="h-8 text-xs"
						onclick={() => handleToggleAll(false)}
						disabled={bulkToggling || kilocodeState.enabledCount === 0}
					>
						Disable All
					</Button>
				</div>

				<!-- Test All button -->
				{#if isTestingAll}
					<Button
						variant="destructive"
						size="sm"
						class="h-8 gap-1.5 text-xs"
						onclick={handleCancelTestAll}
					>
						<SquareIcon class="size-3.5 fill-current" />
						<span>Stop ({testProgress.current}/{testProgress.total})</span>
					</Button>
				{:else}
					<Button
						variant="secondary"
						size="sm"
						class="h-8 gap-1.5 text-xs"
						onclick={handleTestAll}
						disabled={kilocodeState.enabledCount === 0}
					>
						<FlaskConicalIcon class="size-3.5" />
						<span>Test All Active</span>
					</Button>
				{/if}
			</div>
		</Card.Header>

		<Card.Content class="p-0">
			{#if kilocodeState.loading}
				<div class="flex items-center justify-center py-16">
					<RefreshCwIcon class="size-6 animate-spin text-muted-foreground" />
				</div>
			{:else if kilocodeState.filtered.length === 0}
				<div class="flex flex-col items-center justify-center p-8 text-center sm:p-12">
					<p class="text-xs text-muted-foreground">No models found matching your query.</p>
				</div>
			{:else}
				<Table.Root>
					<Table.Header>
						<Table.Row class="hover:bg-transparent">
							<Table.Head class="text-xs">Model</Table.Head>
							<Table.Head class="text-xs">Capabilities</Table.Head>
							<Table.Head class="text-xs">Context Length</Table.Head>
							<Table.Head class="text-xs">Enabled</Table.Head>
							<Table.Head class="text-end text-xs">Test</Table.Head>
						</Table.Row>
					</Table.Header>
					<Table.Body>
						{#each kilocodeState.filtered as m (m.id)}
							<Table.Row class="group">
								<Table.Cell class="py-3">
									<div class="flex flex-col">
										<div class="flex items-center gap-2">
											<span class="font-mono text-xs font-semibold text-foreground">
												{m.id}
											</span>
											{#if m.id.includes(':free') || m.id.includes('free')}
												<Badge variant="outline" class="border-emerald-500/30 bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 font-mono text-[9px] px-1 h-4">
													FREE
												</Badge>
											{/if}
										</div>
									</div>
								</Table.Cell>

								<Table.Cell class="py-3">
									<div class="flex flex-wrap gap-1">
										{#if m.capabilities?.vision}
											<Badge variant="secondary" class="font-mono text-[9px] px-1 py-0 h-4">
												Vision
											</Badge>
										{/if}
										{#if m.capabilities?.reasoning}
											<Badge variant="secondary" class="font-mono text-[9px] px-1 py-0 h-4 text-purple-600 dark:text-purple-400 bg-purple-500/10">
												Reasoning
											</Badge>
										{/if}
										{#if m.capabilities?.tools}
											<Badge variant="secondary" class="font-mono text-[9px] px-1 py-0 h-4 text-blue-600 dark:text-blue-400 bg-blue-500/10">
												Tools
											</Badge>
										{/if}
									</div>
								</Table.Cell>

								<Table.Cell class="py-3">
									{#if m.context_length}
										<span class="font-mono text-xs text-muted-foreground">
											{Math.round(m.context_length / 1000)}k
										</span>
									{:else}
										<span class="text-xs text-muted-foreground">—</span>
									{/if}
								</Table.Cell>

								<Table.Cell class="py-3">
									<Switch
										checked={m.enabled}
										disabled={togglingId === m.id}
										onCheckedChange={(checked) => handleToggleModel(m, checked)}
									/>
								</Table.Cell>

								<Table.Cell class="py-3 text-end">
									<div class="flex items-center justify-end gap-1.5">
										{#if testResults[m.id]}
											<Badge
												variant={testResults[m.id].ok ? 'secondary' : 'destructive'}
												class="font-mono text-[10px]"
											>
												{testResults[m.id].ok ? `${testResults[m.id].latencyMs}ms` : 'Error'}
											</Badge>
										{/if}

										<Button
											variant="ghost"
											size="icon-xs"
											class="text-muted-foreground hover:text-foreground"
											onclick={() => handleTestModel(m.id)}
											disabled={testingModelIds.includes(m.id)}
											title="Ping model"
										>
											<PlayIcon class={`size-3.5 ${testingModelIds.includes(m.id) ? 'animate-spin' : ''}`} />
										</Button>

										<Button
											variant="ghost"
											size="icon-xs"
											class="text-muted-foreground hover:text-primary"
											onclick={() => handleSelectChatModel(m.id)}
											title="Open in Test Chat"
										>
											<MessageSquareIcon class="size-3.5" />
										</Button>
									</div>
								</Table.Cell>
							</Table.Row>
						{/each}
					</Table.Body>
				</Table.Root>
			{/if}
		</Card.Content>
	</Card.Root>
</div>
