<script lang="ts">
	import * as Card from '$lib/components/ui/card';
	import * as Table from '$lib/components/ui/table';
	import * as Select from '$lib/components/ui/select';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Badge } from '$lib/components/ui/badge';
	import { Switch } from '$lib/components/ui/switch';
	import { toast } from '$lib/state/toast.svelte';
	import { cn } from '$lib/utils';
	import type { ProviderModel } from '$lib/types';
	import { pingModel } from '$lib/api/providers';

	import SearchIcon from '@lucide/svelte/icons/search';
	import PlayIcon from '@lucide/svelte/icons/play';
	import FlaskConicalIcon from '@lucide/svelte/icons/flask-conical';
	import MessageSquareIcon from '@lucide/svelte/icons/message-square';
	import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';
	import SquareIcon from '@lucide/svelte/icons/square';

	interface Props {
		title?: string;
		description?: string;
		models: ProviderModel[];
		loading?: boolean;
		onToggleModel: (model: ProviderModel, enabled: boolean) => Promise<void> | void;
		onToggleAll: (enabled: boolean) => Promise<void> | void;
		onSelectChatModel?: (modelId: string) => void;
	}

	let {
		title = 'Available Models',
		description = 'Toggle models on or off. Test models using probe ping.',
		models = [],
		loading = false,
		onToggleModel,
		onToggleAll,
		onSelectChatModel
	}: Props = $props();

	let searchQuery = $state('');
	let filter = $state<'all' | 'enabled' | 'disabled'>('all');
	let togglingId = $state<string | null>(null);
	let bulkToggling = $state(false);

	let testResults = $state<
		Record<string, { ok: boolean; latencyMs?: number; error?: string }>
	>({});
	let testingModelIds = $state<string[]>([]);
	let isTestingAll = $state(false);
	let shouldCancelTestAll = $state(false);
	let testConcurrency = $state(4);
	let testProgress = $state({ current: 0, total: 0 });

	let totalCount = $derived(models.length);
	let enabledCount = $derived(models.filter((m) => m.enabled).length);
	let disabledCount = $derived(models.filter((m) => !m.enabled).length);

	let filtered = $derived(
		models.filter((m) => {
			const matchesSearch =
				searchQuery.trim() === '' ||
				m.id.toLowerCase().includes(searchQuery.toLowerCase()) ||
				(m.name && m.name.toLowerCase().includes(searchQuery.toLowerCase())) ||
				(m.description && m.description.toLowerCase().includes(searchQuery.toLowerCase()));

			const matchesFilter =
				filter === 'all'
					? true
					: filter === 'enabled'
						? m.enabled
						: !m.enabled;

			return matchesSearch && matchesFilter;
		})
	);

	async function handleToggleModel(model: ProviderModel, checked: boolean) {
		togglingId = model.id;
		try {
			await onToggleModel(model, checked);
		} catch (e: unknown) {
			toast.error('Update Failed', e instanceof Error ? e.message : 'Error updating model');
		} finally {
			togglingId = null;
		}
	}

	async function handleToggleAll(enable: boolean) {
		bulkToggling = true;
		try {
			await onToggleAll(enable);
			toast.info('Models Updated', `${enable ? 'Enabled' : 'Disabled'} all models`);
		} catch (e: unknown) {
			toast.error('Bulk Update Failed', e instanceof Error ? e.message : 'Error updating models');
		} finally {
			bulkToggling = false;
		}
	}

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
					toast.error(modelId, res.error || 'Model unreachable');
				}
				return false;
			}
		} catch (e: unknown) {
			const msg = e instanceof Error ? e.message : 'Test failed';
			testResults = {
				...testResults,
				[modelId]: { ok: false, error: msg }
			};
			if (!silent) {
				toast.error(modelId, msg);
			}
			return false;
		} finally {
			testingModelIds = testingModelIds.filter((id) => id !== modelId);
		}
	}

	async function handleTestAll() {
		const targetModels = models.filter((m) => m.enabled);
		if (targetModels.length === 0) {
			toast.warning('No Models', 'No enabled models to test');
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

	function handleSelectChat(modelId: string) {
		onSelectChatModel?.(modelId);
		window.scrollTo({ top: 0, behavior: 'smooth' });
	}
</script>

<Card.Root class="overflow-hidden p-0">
	<Card.Header
		class="flex flex-col gap-4 border-b border-border p-4 sm:flex-row sm:items-center sm:justify-between sm:p-5"
	>
		<div class="flex flex-col gap-1">
			<div class="flex items-center gap-2">
				<Card.Title class="text-base font-semibold">{title}</Card.Title>
				<Badge variant="outline" class="font-mono text-xs">
					{enabledCount} of {totalCount} enabled
				</Badge>
			</div>
			{#if description}
				<Card.Description class="text-xs">
					{description}
				</Card.Description>
			{/if}
		</div>

		<div class="flex flex-wrap items-center gap-2">
			<!-- Search -->
			<div class="relative w-full sm:w-56">
				<SearchIcon
					class="pointer-events-none absolute start-2.5 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground"
				/>
				<Input
					type="search"
					bind:value={searchQuery}
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
						disabled={loading || enabledCount === 0}
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
					disabled={bulkToggling || loading || isTestingAll || enabledCount === totalCount}
					onclick={() => handleToggleAll(true)}
					title="Enable all models"
				>
					Enable All
				</Button>
				<Button
					variant="outline"
					size="sm"
					disabled={bulkToggling || loading || isTestingAll || enabledCount === 0}
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
				filter === 'all'
					? 'bg-background text-foreground shadow-xs'
					: 'text-muted-foreground hover:text-foreground'
			)}
			onclick={() => (filter = 'all')}
		>
			All ({totalCount})
		</button>
		<button
			type="button"
			class={cn(
				'rounded-md px-2.5 py-1 text-xs font-medium transition-colors',
				filter === 'enabled'
					? 'bg-background text-foreground shadow-xs'
					: 'text-muted-foreground hover:text-foreground'
			)}
			onclick={() => (filter = 'enabled')}
		>
			Enabled ({enabledCount})
		</button>
		<button
			type="button"
			class={cn(
				'rounded-md px-2.5 py-1 text-xs font-medium transition-colors',
				filter === 'disabled'
					? 'bg-background text-foreground shadow-xs'
					: 'text-muted-foreground hover:text-foreground'
			)}
			onclick={() => (filter = 'disabled')}
		>
			Disabled ({disabledCount})
		</button>
	</div>

	<!-- Models Table -->
	<Card.Content class="overflow-x-auto p-0">
		<Table.Root>
			<Table.Header>
				<Table.Row class="hover:bg-transparent">
					<Table.Head>MODEL ID</Table.Head>
					<Table.Head class="w-[120px]">ENABLED</Table.Head>
					<Table.Head class="w-[180px] text-end">ACTIONS</Table.Head>
				</Table.Row>
			</Table.Header>
			<Table.Body>
				{#if loading}
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
				{:else if filtered.length === 0}
					<Table.Row>
						<Table.Cell colspan={3} class="h-32 text-center text-muted-foreground">
							<div class="flex flex-col items-center justify-center gap-1">
								<p class="text-xs font-medium text-foreground">No models found</p>
								<p class="text-xs">No models match the filter criteria.</p>
							</div>
						</Table.Cell>
					</Table.Row>
				{:else}
					{#each filtered as m (m.id)}
						<Table.Row class={cn(!m.enabled && 'bg-muted/10 opacity-60')}>
							<Table.Cell class="font-mono text-xs">
								<div class="flex flex-col gap-1">
									<div class="flex flex-wrap items-center gap-1.5">
										<span class="font-semibold text-foreground">{m.id}</span>
										{#if m.id.includes(':free') || m.id.includes('free')}
											<Badge
												variant="outline"
												class="border-emerald-500/30 bg-emerald-500/10 px-1.5 py-0 text-[10px] font-normal text-emerald-600 dark:text-emerald-400"
											>
												Free
											</Badge>
										{/if}
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
										{#if m.capabilities?.reasoning}
											<Badge
												variant="outline"
												class="border-purple-500/30 bg-purple-500/10 px-1.5 py-0 text-[10px] font-normal text-purple-600 dark:text-purple-400"
											>
												Reasoning
											</Badge>
										{/if}
										{#if m.capabilities?.tools}
											<Badge
												variant="outline"
												class="border-blue-500/30 bg-blue-500/10 px-1.5 py-0 text-[10px] font-normal text-blue-600 dark:text-blue-400"
											>
												Tools
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
												title={testResults[m.id].error}
											>
												Fail
											</Badge>
										{/if}
									{/if}

									{#if onSelectChatModel}
										<Button
											variant="ghost"
											size="sm"
											class="h-7 px-2 text-xs text-muted-foreground hover:bg-muted hover:text-foreground"
											onclick={() => handleSelectChat(m.id)}
											title={`Select ${m.id} in temporary chat`}
										>
											<MessageSquareIcon data-icon="inline-start" class="size-3" />
											Chat
										</Button>
									{/if}

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
