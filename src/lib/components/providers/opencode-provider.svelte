<script lang="ts">
	import * as Card from '$lib/components/ui/card';
	import * as Table from '$lib/components/ui/table';
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

	interface Props {
		providerState: ProviderState;
	}

	let { providerState }: Props = $props();

	let selectedChatModel = $state('big-pickle');
	let isTestingAll = $state(false);
	let testProgress = $state({ current: 0, total: 0 });
	let testingModelId = $state<string | null>(null);
	let shouldCancelTestAll = false;

	let togglingId = $state<string | null>(null);
	let bulkToggling = $state(false);
	let checkingVersion = $state(false);

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
		testingModelId = modelId;

		try {
			const res = await pingModel(modelId, '');
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
			if (!silent) {
				toast.error(modelId, e instanceof Error ? e.message : 'Network error');
			}
			return false;
		} finally {
			if (testingModelId === modelId) {
				testingModelId = null;
			}
		}
	}

	// Test all enabled models sequentially
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

		for (let i = 0; i < targetModels.length; i++) {
			if (shouldCancelTestAll) break;

			const m = targetModels[i];
			testProgress = { current: i + 1, total: targetModels.length };

			const ok = await handleTestModel(m.id, true);
			if (ok) passed++;
			else failed++;

			// Small 250ms pause between pings like 9router
			if (i < targetModels.length - 1 && !shouldCancelTestAll) {
				await new Promise((r) => setTimeout(r, 250));
			}
		}

		isTestingAll = false;
		testingModelId = null;

		if (!shouldCancelTestAll) {
			if (failed === 0) {
				toast.success('Test All Completed', `All ${passed} models reachable`);
			} else {
				toast.error('Test All Completed', `${passed} passed, ${failed} failed`);
			}
		}
	}

	function handleStopTestAll() {
		shouldCancelTestAll = true;
		isTestingAll = false;
		testingModelId = null;
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

				<!-- Test All button (9router style) -->
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
						title="Test all enabled models with empty prompt completions ping"
					>
						<FlaskConicalIcon data-icon="inline-start" class="size-3.5" />
						Test All
					</Button>
				{/if}

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
		<Card.Content class="p-0">
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
											disabled={testingModelId === m.id || isTestingAll}
											onclick={() => handleTestModel(m.id)}
											title={`Send empty prompt ping to ${m.id}`}
										>
											{#if testingModelId === m.id}
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
