<script lang="ts">
	import * as Card from '$lib/components/ui/card';
	import * as Table from '$lib/components/ui/table';
	import { Button } from '$lib/components/ui/button';
	import { Badge } from '$lib/components/ui/badge';
	import { Switch } from '$lib/components/ui/switch';
	import { cn } from '$lib/utils';
	import { toast } from '$lib/state/toast.svelte';
	import type { AntigravityState } from '$lib/state/antigravity.svelte';
	import AddAccountDialog from './add-account-dialog.svelte';
	import type { CreateAccountPayload } from '$lib/api/antigravity';

	import PlusIcon from '@lucide/svelte/icons/plus';
	import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';
	import Trash2Icon from '@lucide/svelte/icons/trash-2';
	import UserIcon from '@lucide/svelte/icons/user';

	interface Props {
		antigravityState: AntigravityState;
	}

	let { antigravityState }: Props = $props();

	let dialogOpen = $state(false);
	let togglingId = $state<string | null>(null);
	let refreshingId = $state<string | null>(null);
	let deletingId = $state<string | null>(null);

	async function handleAddAccount(payload: CreateAccountPayload) {
		try {
			const acc = await antigravityState.addAccount(payload);
			toast.success('Account Connected', `Added ${acc.email} (${acc.projectId})`);
		} catch (e: unknown) {
			toast.error('Add Failed', e instanceof Error ? e.message : 'Error adding account');
			throw e;
		}
	}

	async function handleToggleAccount(id: string, active: boolean) {
		try {
			togglingId = id;
			await antigravityState.toggleAccount(id, active);
			toast.info('Account Updated', `Account ${active ? 'activated' : 'deactivated'}`);
		} catch (e: unknown) {
			toast.error('Toggle Failed', e instanceof Error ? e.message : 'Error updating account');
		} finally {
			togglingId = null;
		}
	}

	async function handleRefreshAccount(id: string) {
		try {
			refreshingId = id;
			const res = await antigravityState.refreshAccount(id);
			if (res.ok) {
				toast.success('Token Refreshed', 'OAuth token refreshed with Google');
			} else {
				toast.error('Refresh Failed', res.error || 'Failed to refresh token');
			}
		} catch (e: unknown) {
			toast.error('Refresh Error', e instanceof Error ? e.message : 'Network error');
		} finally {
			refreshingId = null;
		}
	}

	async function handleDeleteAccount(id: string) {
		if (!confirm('Are you sure you want to remove this account?')) return;
		try {
			deletingId = id;
			await antigravityState.deleteAccount(id);
			toast.info('Account Removed', 'Account deleted from router');
		} catch (e: unknown) {
			toast.error('Delete Failed', e instanceof Error ? e.message : 'Error deleting account');
		} finally {
			deletingId = null;
		}
	}
</script>

<Card.Root class="overflow-hidden p-0">
	<Card.Header
		class="flex flex-col gap-4 border-b border-border p-4 sm:flex-row sm:items-center sm:justify-between sm:p-5"
	>
		<div class="flex flex-col gap-1">
			<div class="flex items-center gap-2">
				<Card.Title class="text-base font-semibold">Google Accounts</Card.Title>
				<Badge variant="outline" class="font-mono text-xs">
					{antigravityState.activeAccountsCount} active / {antigravityState.totalAccountsCount} total
				</Badge>
			</div>
			<Card.Description class="text-xs">
				Manage Google OAuth accounts used to execute Antigravity completions.
			</Card.Description>
		</div>

		<div class="flex items-center gap-2">
			<Button
				variant="outline"
				size="sm"
				disabled={antigravityState.accountsLoading}
				onclick={() => antigravityState.loadAccounts()}
			>
				<RefreshCwIcon
					data-icon="inline-start"
					class={cn('size-3.5', antigravityState.accountsLoading && 'animate-spin')}
				/>
				Refresh
			</Button>

			<Button variant="default" size="sm" onclick={() => (dialogOpen = true)}>
				<PlusIcon data-icon="inline-start" class="size-3.5" />
				Add Account
			</Button>
		</div>
	</Card.Header>

	<Card.Content class="p-0">
		<Table.Root>
			<Table.Header>
				<Table.Row class="hover:bg-transparent">
					<Table.Head>ACCOUNT / EMAIL</Table.Head>
					<Table.Head>PROJECT ID</Table.Head>
					<Table.Head class="w-[120px]">STATUS</Table.Head>
					<Table.Head class="w-[120px]">QUOTA</Table.Head>
					<Table.Head class="w-[110px] text-end">ACTIONS</Table.Head>
				</Table.Row>
			</Table.Header>
			<Table.Body>
				{#if antigravityState.accountsLoading}
					<Table.Row>
						<Table.Cell colspan={5} class="h-28 text-center text-muted-foreground">
							<div class="flex flex-col items-center justify-center gap-2">
								<div
									class="size-4 animate-spin rounded-full border-2 border-primary border-t-transparent"
								></div>
								<span class="text-xs">Loading accounts...</span>
							</div>
						</Table.Cell>
					</Table.Row>
				{:else if antigravityState.accounts.length === 0}
					<Table.Row>
						<Table.Cell colspan={5} class="h-28 text-center text-muted-foreground">
							<div class="flex flex-col items-center justify-center gap-1.5">
								<p class="text-xs font-medium text-foreground">No accounts connected</p>
								<p class="text-xs">
									Click "Add Account" to connect your Google account via OAuth or paste credentials.
								</p>
							</div>
						</Table.Cell>
					</Table.Row>
				{:else}
					{#each antigravityState.accounts as acc (acc.id)}
						<Table.Row class={cn(!acc.isActive && 'bg-muted/10 opacity-60')}>
							<Table.Cell>
								<div class="flex items-center gap-2.5">
									<div
										class="flex size-7 items-center justify-center rounded-full bg-primary/10 text-primary"
									>
										<UserIcon class="size-3.5" />
									</div>
									<div class="flex flex-col">
										<span class="font-medium text-foreground">{acc.email}</span>
										<span class="font-mono text-[10px] text-muted-foreground">{acc.id}</span>
									</div>
								</div>
							</Table.Cell>

							<Table.Cell class="font-mono text-xs">
								<span class="text-foreground">{acc.projectId}</span>
							</Table.Cell>

							<Table.Cell>
								<div class="flex items-center gap-2">
									<Switch
										checked={acc.isActive}
										disabled={togglingId === acc.id}
										onCheckedChange={(checked) => handleToggleAccount(acc.id, checked)}
										size="sm"
										aria-label={`Toggle account ${acc.email}`}
									/>
									<Badge
										variant={acc.isActive ? 'default' : 'secondary'}
										class="text-[10px] font-medium"
									>
										{acc.isActive ? 'Active' : 'Inactive'}
									</Badge>
								</div>
							</Table.Cell>

							<Table.Cell>
								{@const usage = antigravityState.usages[acc.id]}
								{@const quota =
									usage?.quotas?.['gemini_session'] ||
									usage?.quotas?.['gemini_weekly'] ||
									usage?.quotas?.['claude_gpt_session'] ||
									usage?.quotas?.['claude_gpt_weekly']}
								{#if quota}
									{@const pct = quota.remainingPercentage}
									<div class="flex items-center gap-1.5 font-mono text-xs">
										<span>{pct > 70 ? '🟢' : pct >= 30 ? '🟡' : '🔴'}</span>
										<span
											class={cn(
												'font-medium',
												pct > 70
													? 'text-emerald-500'
													: pct >= 30
														? 'text-amber-500'
														: 'text-destructive'
											)}
										>
											{pct.toFixed(0)}%
										</span>
										<span class="text-[10px] text-muted-foreground">
											({quota.name.includes('Weekly') ? 'W' : '5h'})
										</span>
									</div>
								{:else if antigravityState.quotaLoading}
									<span class="animate-pulse text-xs text-muted-foreground">...</span>
								{:else}
									<span class="text-xs text-muted-foreground">—</span>
								{/if}
							</Table.Cell>

							<Table.Cell class="text-end">
								<div class="flex items-center justify-end gap-1">
									<Button
										variant="ghost"
										size="icon-sm"
										disabled={refreshingId === acc.id}
										onclick={() => handleRefreshAccount(acc.id)}
										title="Refresh Google OAuth Token"
									>
										<RefreshCwIcon
											class={cn('size-3.5', refreshingId === acc.id && 'animate-spin')}
										/>
									</Button>
									<Button
										variant="ghost"
										size="icon-sm"
										class="text-destructive hover:bg-destructive/10"
										disabled={deletingId === acc.id}
										onclick={() => handleDeleteAccount(acc.id)}
										title="Delete Account"
									>
										<Trash2Icon class="size-3.5" />
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

<AddAccountDialog
	bind:open={dialogOpen}
	onAdd={handleAddAccount}
	onClose={() => (dialogOpen = false)}
/>
