<script lang="ts">
	import * as Card from '$lib/components/ui/card';
	import { Button } from '$lib/components/ui/button';
	import { Badge } from '$lib/components/ui/badge';
	import { cn } from '$lib/utils';
	import { toast } from '$lib/state/toast.svelte';
	import type { AccountUsage, QuotaItem } from '$lib/types';

	import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';
	import ActivityIcon from '@lucide/svelte/icons/activity';
	import SparklesIcon from '@lucide/svelte/icons/sparkles';
	import ChevronDownIcon from '@lucide/svelte/icons/chevron-down';
	import ChevronUpIcon from '@lucide/svelte/icons/chevron-up';
	import ClockIcon from '@lucide/svelte/icons/clock';

	export interface QuotaAccount {
		id: string;
		email?: string;
		isActive: boolean;
		subtitle?: string;
		projectId?: string;
		orgName?: string;
	}

	interface Props {
		accounts: QuotaAccount[];
		usages: Record<string, AccountUsage>;
		loading: boolean;
		summaryKeys: string[];
		onRefreshAll: () => Promise<void>;
		onRefreshAccount: (id: string) => Promise<void>;
		title?: string;
		description?: string;
		emptyTitle?: string;
		emptyText?: string;
		loadingText?: string;
		noMetricsText?: string;
		defaultPlan?: string;
		accent?: 'primary' | 'orange';
		subtitleFor?: (account: QuotaAccount) => string;
		refreshAllSuccess?: string;
		refreshAccountSuccess?: string;
	}

	let {
		accounts,
		usages,
		loading,
		summaryKeys,
		onRefreshAll,
		onRefreshAccount,
		title = 'Quota Tracker',
		description = '',
		emptyTitle = 'No Accounts Connected',
		emptyText = '',
		loadingText = 'Loading quota information...',
		noMetricsText = 'No quota metrics recorded yet.',
		defaultPlan = 'Standard Tier',
		accent = 'primary',
		subtitleFor,
		refreshAllSuccess = 'Quotas Refreshed',
		refreshAccountSuccess = 'Quota Updated'
	}: Props = $props();

	let refreshingAccountId = $state<string | null>(null);
	let expandedModels = $state<Record<string, boolean>>({});

	const accentClasses = $derived(
		accent === 'orange'
			? 'border-orange-500/20 bg-orange-500/10 text-orange-500'
			: 'border-primary/20 bg-primary/10 text-primary'
	);

	function toggleExpanded(accountId: string) {
		expandedModels[accountId] = !expandedModels[accountId];
	}

	async function handleRefreshAll() {
		try {
			await onRefreshAll();
			toast.success(refreshAllSuccess, 'Fetched latest quota usage');
		} catch (e: unknown) {
			toast.error('Quota Refresh Failed', e instanceof Error ? e.message : 'Error fetching quota');
		}
	}

	async function handleRefreshAccount(accountId: string) {
		try {
			refreshingAccountId = accountId;
			await onRefreshAccount(accountId);
			toast.success(refreshAccountSuccess, 'Refreshed quota for this account');
		} catch (e: unknown) {
			toast.error('Refresh Failed', e instanceof Error ? e.message : 'Failed to fetch quota');
		} finally {
			refreshingAccountId = null;
		}
	}

	function formatResetCountdown(dateStr?: string | null): string {
		if (!dateStr) return '-';
		try {
			const resetDate = new Date(dateStr);
			const diffMs = resetDate.getTime() - Date.now();
			if (diffMs <= 0) return '-';

			const totalMinutes = Math.ceil(diffMs / 60000);
			if (totalMinutes < 60) {
				return `${totalMinutes}m`;
			}
			const hours = Math.floor(totalMinutes / 60);
			const mins = totalMinutes % 60;
			if (hours < 24) {
				return `${hours}h ${mins}m`;
			}
			const days = Math.floor(hours / 24);
			const remHours = hours % 24;
			return `${days}d ${remHours}h ${mins}m`;
		} catch {
			return '-';
		}
	}

	function formatResetDisplay(dateStr?: string | null): string | null {
		if (!dateStr) return null;
		try {
			const date = new Date(dateStr);
			const diffHours = (date.getTime() - Date.now()) / (1000 * 60 * 60);
			const day =
				diffHours < 24
					? 'Today'
					: diffHours < 48
						? 'Tomorrow'
						: date.toLocaleDateString('en-US', { month: 'short', day: 'numeric' });

			const time = date.toLocaleTimeString('en-US', {
				hour: 'numeric',
				minute: '2-digit',
				hour12: true
			});

			return `${day}, ${time}`;
		} catch {
			return null;
		}
	}

	function getQuotaColors(remaining: number) {
		if (remaining > 70) {
			return {
				text: 'text-emerald-500',
				bg: 'bg-emerald-500',
				bgLight: 'bg-emerald-500/10',
				border: 'border-emerald-500/20',
				emoji: '🟢'
			};
		}
		if (remaining >= 30) {
			return {
				text: 'text-amber-500',
				bg: 'bg-amber-500',
				bgLight: 'bg-amber-500/10',
				border: 'border-amber-500/20',
				emoji: '🟡'
			};
		}
		return {
			text: 'text-destructive',
			bg: 'bg-destructive',
			bgLight: 'bg-destructive/10',
			border: 'border-destructive/20',
			emoji: '🔴'
		};
	}

	function separateQuotas(quotas: Record<string, QuotaItem> = {}) {
		const summary: QuotaItem[] = [];
		const models: QuotaItem[] = [];

		for (const key of summaryKeys) {
			if (quotas[key]) {
				summary.push(quotas[key]);
			}
		}

		for (const [k, q] of Object.entries(quotas)) {
			if (!summaryKeys.includes(k)) {
				models.push(q);
			}
		}

		return { summary, models };
	}
</script>

<Card.Root class="overflow-hidden p-0">
	<Card.Header
		class="flex flex-col gap-4 border-b border-border p-4 sm:flex-row sm:items-center sm:justify-between sm:p-5"
	>
		<div class="flex items-center gap-3">
			<div
				class={cn(
					'flex size-9 items-center justify-center rounded-lg border shadow-xs',
					accentClasses
				)}
			>
				<ActivityIcon class="size-4" />
			</div>
			<div class="flex flex-col gap-0.5">
				<div class="flex items-center gap-2">
					<Card.Title class="text-base font-semibold">{title}</Card.Title>
					<Badge variant="outline" class="font-mono text-xs">
						{accounts.length}
						{accounts.length === 1 ? 'account' : 'accounts'}
					</Badge>
				</div>
				<Card.Description class="text-xs text-muted-foreground">
					{description}
				</Card.Description>
			</div>
		</div>

		<div class="flex items-center gap-2">
			<Button
				variant="outline"
				size="sm"
				disabled={loading || accounts.length === 0}
				onclick={handleRefreshAll}
			>
				<RefreshCwIcon data-icon="inline-start" class={cn('size-3.5', loading && 'animate-spin')} />
				Refresh Quotas
			</Button>
		</div>
	</Card.Header>

	<Card.Content class="p-4 sm:p-5">
		{#if accounts.length === 0}
			<div class="flex flex-col items-center justify-center py-10 text-center">
				<div
					class="flex size-12 items-center justify-center rounded-full bg-muted text-muted-foreground"
				>
					<ActivityIcon class="size-6" />
				</div>
				<h3 class="mt-3 text-sm font-medium text-foreground">{emptyTitle}</h3>
				<p class="mt-1 max-w-sm text-xs text-muted-foreground">
					{emptyText}
				</p>
			</div>
		{:else}
			<div class="flex flex-col gap-5">
				{#each accounts as account (account.id)}
					{@const usage = usages[account.id]}
					{@const isRefreshing = refreshingAccountId === account.id}
					{@const { summary, models } = separateQuotas(usage?.quotas)}
					{@const subtitle = subtitleFor ? subtitleFor(account) : account.subtitle}

					<div class="flex flex-col rounded-xl border border-border bg-card p-4 transition-all">
						<!-- Account Header Bar -->
						<div
							class="flex flex-col gap-2 border-b border-border/60 pb-3 sm:flex-row sm:items-center sm:justify-between"
						>
							<div class="flex items-center gap-2.5">
								<span
									class={cn(
										'size-2.5 shrink-0 rounded-full',
										account.isActive ? 'bg-emerald-500' : 'bg-muted-foreground/40'
									)}
								></span>
								<div class="flex flex-col">
									<div class="flex items-center gap-2">
										<span class="text-sm font-medium text-foreground">
											{account.email || 'Account'}
										</span>
										<Badge variant="secondary" class="h-4 px-1.5 font-mono text-[10px]">
											{usage?.plan || defaultPlan}
										</Badge>
										{#if !account.isActive}
											<Badge variant="outline" class="h-4 px-1.5 text-[10px] text-muted-foreground">
												Inactive
											</Badge>
										{/if}
									</div>
									{#if subtitle}
										<span class="font-mono text-[11px] text-muted-foreground">
											{subtitle}
										</span>
									{/if}
								</div>
							</div>

							<div class="flex items-center gap-2 self-end sm:self-auto">
								{#if usage?.fetchedAt}
									<span class="text-[10px] text-muted-foreground">
										Updated {new Date(usage.fetchedAt).toLocaleTimeString([], {
											hour: '2-digit',
											minute: '2-digit'
										})}
									</span>
								{/if}
								<Button
									variant="ghost"
									size="icon-xs"
									disabled={isRefreshing || loading}
									onclick={() => handleRefreshAccount(account.id)}
									title="Refresh account quota"
								>
									<RefreshCwIcon class={cn('size-3.5', isRefreshing && 'animate-spin')} />
								</Button>
							</div>
						</div>

						<!-- Quotas Section -->
						<div class="mt-3.5 flex flex-col gap-3">
							{#if !usage && loading}
								<div
									class="flex animate-pulse items-center gap-2 py-4 text-xs text-muted-foreground"
								>
									<RefreshCwIcon class="size-4 animate-spin" />
									{loadingText}
								</div>
							{:else if usage?.message && !summary.length && !models.length}
								<div class="py-3 text-xs text-muted-foreground italic">{usage.message}</div>
							{:else if !usage || (!summary.length && !models.length)}
								<div class="py-3 text-xs text-muted-foreground italic">{noMetricsText}</div>
							{:else}
								<!-- Primary Summary Buckets -->
								{#if summary.length > 0}
									<div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
										{#each summary as item (item.modelKey || item.name)}
											{@const colors = getQuotaColors(item.remainingPercentage)}
											{@const countdown = formatResetCountdown(item.resetAt)}
											{@const resetDisplay = formatResetDisplay(item.resetAt)}

											<div
												class="flex flex-col gap-2 rounded-lg border border-border/70 bg-muted/20 p-3"
											>
												<div class="flex items-center justify-between">
													<div class="flex min-w-0 items-center gap-1.5">
														<span class="shrink-0 text-xs">{colors.emoji}</span>
														<span class="truncate text-xs font-semibold text-foreground">
															{item.name}
														</span>
													</div>
													<span class={cn('shrink-0 font-mono text-xs font-bold', colors.text)}>
														{item.remainingPercentage.toFixed(0)}%
													</span>
												</div>

												<div
													class={cn(
														'h-2 w-full overflow-hidden rounded-full border',
														colors.bgLight,
														colors.border
													)}
												>
													<div
														class={cn('h-full transition-all duration-300', colors.bg)}
														style="width: {Math.max(0, Math.min(100, item.remainingPercentage))}%"
													></div>
												</div>

												<div
													class="flex items-center justify-between text-[11px] text-muted-foreground"
												>
													<span
														>{item.used.toLocaleString()} / {item.total.toLocaleString()} used</span
													>
													{#if countdown !== '-'}
														<div class="flex items-center gap-1 font-medium text-foreground/80">
															<ClockIcon class="size-3 text-muted-foreground" />
															<span>Resets in {countdown}</span>
														</div>
													{:else if resetDisplay}
														<span class="text-[10px]">{resetDisplay}</span>
													{/if}
												</div>
											</div>
										{/each}
									</div>
								{/if}

								<!-- Collapsible Individual Models Details -->
								{#if models.length > 0}
									<div class="mt-1 flex flex-col gap-2">
										<button
											type="button"
											class="flex items-center justify-between rounded-md py-1.5 text-xs font-medium text-muted-foreground transition-colors hover:text-foreground"
											onclick={() => toggleExpanded(account.id)}
										>
											<div class="flex items-center gap-1.5">
												<SparklesIcon class="size-3.5" />
												<span>Individual Models Quotas ({models.length})</span>
											</div>
											{#if expandedModels[account.id]}
												<ChevronUpIcon class="size-3.5" />
											{:else}
												<ChevronDownIcon class="size-3.5" />
											{/if}
										</button>

										{#if expandedModels[account.id]}
											<div class="grid grid-cols-1 gap-2.5 pt-1 sm:grid-cols-2 md:grid-cols-3">
												{#each models as modelItem (modelItem.modelKey || modelItem.name)}
													{@const colors = getQuotaColors(modelItem.remainingPercentage)}
													{@const countdown = formatResetCountdown(modelItem.resetAt)}

													<div
														class="flex flex-col gap-1.5 rounded-lg border border-border/50 bg-background/50 p-2.5"
													>
														<div class="flex items-center justify-between gap-1">
															<span
																class="truncate text-[11px] font-medium text-foreground"
																title={modelItem.name}
															>
																{modelItem.name}
															</span>
															<span
																class={cn(
																	'shrink-0 font-mono text-[10px] font-semibold',
																	colors.text
																)}
															>
																{modelItem.remainingPercentage.toFixed(0)}%
															</span>
														</div>

														<div
															class={cn(
																'h-1.5 w-full overflow-hidden rounded-full',
																colors.bgLight
															)}
														>
															<div
																class={cn('h-full transition-all duration-300', colors.bg)}
																style="width: {Math.max(
																	0,
																	Math.min(100, modelItem.remainingPercentage)
																)}%"
															></div>
														</div>

														<div
															class="flex items-center justify-between text-[10px] text-muted-foreground"
														>
															<span>{modelItem.used}/{modelItem.total}</span>
															{#if countdown !== '-'}
																<span>in {countdown}</span>
															{/if}
														</div>
													</div>
												{/each}
											</div>
										{/if}
									</div>
								{/if}
							{/if}
						</div>
					</div>
				{/each}
			</div>
		{/if}
	</Card.Content>
</Card.Root>
