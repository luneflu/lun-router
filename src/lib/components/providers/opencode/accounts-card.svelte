<script lang="ts">
	import * as Card from '$lib/components/ui/card';
	import * as Table from '$lib/components/ui/table';
	import { Button } from '$lib/components/ui/button';
	import { Badge } from '$lib/components/ui/badge';
	import { Switch } from '$lib/components/ui/switch';
	import { toast } from '$lib/state/toast.svelte';
	import type { ProviderState } from '$lib/state/provider.svelte';
	import AddAccountDialog from './add-account-dialog.svelte';

	import PlusIcon from '@lucide/svelte/icons/plus';
	import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';
	import Trash2Icon from '@lucide/svelte/icons/trash-2';
	import UserIcon from '@lucide/svelte/icons/user';
	import PlayIcon from '@lucide/svelte/icons/play';

	interface Props {
		providerState: ProviderState;
	}

	let { providerState }: Props = $props();

	let dialogOpen = $state(false);
	let togglingId = $state<string | null>(null);
	let testingId = $state<string | null>(null);
	let deletingId = $state<string | null>(null);
	let testResults = $state<Record<string, { ok: boolean; latency_ms?: number; error?: string }>>(
		{}
	);

	function formatExpiry(value?: string): string {
		if (!value) return '—';
		const date = new Date(value);
		if (Number.isNaN(date.getTime())) return '—';
		return date.toLocaleString(undefined, {
			month: 'short',
			day: 'numeric',
			hour: '2-digit',
			minute: '2-digit'
		});
	}

	async function handleToggleAccount(id: string, active: boolean) {
		try {
			togglingId = id;
			await providerState.toggleAccount(id, active);
			toast.info('Account Updated', `Account ${active ? 'activated' : 'deactivated'}`);
			await providerState.load();
		} catch (e: unknown) {
			toast.error('Toggle Failed', e instanceof Error ? e.message : 'Error updating account');
		} finally {
			togglingId = null;
		}
	}

	async function handleTestAccount(id: string) {
		try {
			testingId = id;
			const res = await providerState.testAccount(id);
			testResults = { ...testResults, [id]: res };
			if (res.ok) {
				toast.success('Account Reachable', `Latency: ${res.latency_ms}ms`);
			} else {
				toast.error('Test Failed', res.error || 'OpenCode login is invalid');
			}
		} catch (e: unknown) {
			const errMsg = e instanceof Error ? e.message : 'Network error';
			testResults = { ...testResults, [id]: { ok: false, error: errMsg } };
			toast.error('Test Error', errMsg);
		} finally {
			testingId = null;
		}
	}

	async function handleDeleteAccount(id: string) {
		if (!confirm('Are you sure you want to remove this OpenCode Zen key?')) return;
		try {
			deletingId = id;
			await providerState.deleteAccount(id);
			toast.info('Account Removed', 'Login deleted from router');
			await providerState.load();
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
				<Card.Title class="text-base font-semibold">OpenCode Zen Accounts</Card.Title>
				<Badge variant="outline" class="font-mono text-xs">
					{providerState.activeAccountsCount} active / {providerState.totalAccountsCount} total
				</Badge>
			</div>
			<Card.Description class="text-xs">
				OpenCode Console OAuth logins. Paid models use the OpenCode Go lane; free models and an
				empty account list use the anonymous <code class="font-mono">Bearer public</code> free tier.
			</Card.Description>
		</div>

		<div class="flex items-center gap-2">
			<Button
				variant="outline"
				size="sm"
				class="gap-1.5 text-xs"
				onclick={() => providerState.loadAccounts()}
				disabled={providerState.accountsLoading}
			>
				<RefreshCwIcon class={`size-3.5 ${providerState.accountsLoading ? 'animate-spin' : ''}`} />
				<span>Refresh</span>
			</Button>

			<Button size="sm" class="gap-1.5 text-xs" onclick={() => (dialogOpen = true)}>
				<PlusIcon class="size-3.5" />
				<span>Connect Account</span>
			</Button>
		</div>
	</Card.Header>

	<Card.Content class="p-0">
		{#if providerState.accountsLoading && providerState.accounts.length === 0}
			<div class="flex items-center justify-center py-12">
				<RefreshCwIcon class="size-6 animate-spin text-muted-foreground" />
			</div>
		{:else if providerState.accounts.length === 0}
			<div class="flex flex-col items-center justify-center p-8 text-center sm:p-12">
				<div
					class="mb-3 flex size-12 items-center justify-center rounded-2xl bg-orange-500/10 text-orange-500"
				>
					<UserIcon class="size-6" />
				</div>
				<h4 class="text-sm font-semibold text-foreground">No OpenCode Account</h4>
				<p class="mt-1 max-w-sm text-xs text-muted-foreground">
					Connect an OpenCode Console account to use OpenCode Go models through
					<code class="font-mono">/zen/go/v1</code>. Without an account, requests stay on the
					anonymous free tier.
				</p>
				<Button size="sm" class="mt-4 gap-1.5 text-xs" onclick={() => (dialogOpen = true)}>
					<PlusIcon class="size-3.5" />
					<span>Connect First Account</span>
				</Button>
			</div>
		{:else}
			<Table.Root>
				<Table.Header>
					<Table.Row class="hover:bg-transparent">
						<Table.Head class="text-xs">Account</Table.Head>
						<Table.Head class="text-xs">Organization</Table.Head>
						<Table.Head class="text-xs">Token Expiry</Table.Head>
						<Table.Head class="text-xs">Status</Table.Head>
						<Table.Head class="text-end text-xs">Actions</Table.Head>
					</Table.Row>
				</Table.Header>
				<Table.Body>
					{#each providerState.accounts as acc (acc.id)}
						<Table.Row class="group">
							<Table.Cell class="py-3">
								<div class="flex items-center gap-2.5">
									<div
										class="flex size-7 shrink-0 items-center justify-center rounded-full bg-orange-500/10 font-mono text-xs font-semibold text-orange-500"
									>
										{acc.email ? acc.email.charAt(0).toUpperCase() : 'O'}
									</div>
									<div class="flex flex-col">
										<span class="text-xs font-medium text-foreground">{acc.email || acc.id}</span>
										<span class="font-mono text-[10px] text-muted-foreground">{acc.id}</span>
									</div>
								</div>
							</Table.Cell>

							<Table.Cell class="py-3">
								<span class="text-xs text-muted-foreground">{acc.orgName || acc.orgId || '—'}</span>
							</Table.Cell>

							<Table.Cell class="py-3">
								<span class="font-mono text-[11px] text-muted-foreground">
									{formatExpiry(acc.expiresAt)}
								</span>
							</Table.Cell>

							<Table.Cell class="py-3">
								<div class="flex items-center gap-2">
									<Switch
										checked={acc.isActive}
										disabled={togglingId === acc.id}
										onCheckedChange={(checked) => handleToggleAccount(acc.id, checked)}
									/>
									<span class="text-[11px] text-muted-foreground">
										{acc.isActive ? 'Active' : 'Paused'}
									</span>
								</div>
							</Table.Cell>

							<Table.Cell class="py-3 text-end">
								<div class="flex items-center justify-end gap-1.5">
									{#if testResults[acc.id]}
										<Badge
											variant={testResults[acc.id].ok ? 'secondary' : 'destructive'}
											class="font-mono text-[10px]"
										>
											{testResults[acc.id].ok ? `${testResults[acc.id].latency_ms}ms` : 'Error'}
										</Badge>
									{/if}

									<Button
										variant="ghost"
										size="icon-xs"
										class="text-muted-foreground hover:text-foreground"
										onclick={() => handleTestAccount(acc.id)}
										disabled={testingId === acc.id}
										title="Test account against /api/user"
									>
										<PlayIcon class={`size-3.5 ${testingId === acc.id ? 'animate-spin' : ''}`} />
									</Button>

									<Button
										variant="ghost"
										size="icon-xs"
										class="text-muted-foreground hover:text-destructive"
										onclick={() => handleDeleteAccount(acc.id)}
										disabled={deletingId === acc.id}
										title="Remove key"
									>
										<Trash2Icon class="size-3.5" />
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

<AddAccountDialog
	bind:open={dialogOpen}
	onSuccess={async () => {
		await providerState.load();
	}}
	onClose={() => (dialogOpen = false)}
/>
