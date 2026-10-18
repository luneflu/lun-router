<script lang="ts">
	import * as Card from '$lib/components/ui/card';
	import * as Table from '$lib/components/ui/table';
	import { Button } from '$lib/components/ui/button';
	import { Badge } from '$lib/components/ui/badge';
	import { Switch } from '$lib/components/ui/switch';
	import { toast } from '$lib/state/toast.svelte';
	import type { KilocodeState } from '$lib/state/kilocode.svelte';
	import AddAccountDialog from './add-account-dialog.svelte';

	import PlusIcon from '@lucide/svelte/icons/plus';
	import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';
	import Trash2Icon from '@lucide/svelte/icons/trash-2';
	import UserIcon from '@lucide/svelte/icons/user';
	import PlayIcon from '@lucide/svelte/icons/play';

	interface Props {
		kilocodeState: KilocodeState;
	}

	let { kilocodeState }: Props = $props();

	let dialogOpen = $state(false);
	let togglingId = $state<string | null>(null);
	let testingId = $state<string | null>(null);
	let deletingId = $state<string | null>(null);
	let testResults = $state<Record<string, { ok: boolean; latency_ms?: number; error?: string }>>({});

	async function handleToggleAccount(id: string, active: boolean) {
		try {
			togglingId = id;
			await kilocodeState.toggleAccount(id, active);
			toast.info('Account Updated', `Account ${active ? 'activated' : 'deactivated'}`);
		} catch (e: unknown) {
			toast.error('Toggle Failed', e instanceof Error ? e.message : 'Error updating account');
		} finally {
			togglingId = null;
		}
	}

	async function handleTestAccount(id: string) {
		try {
			testingId = id;
			const res = await kilocodeState.testAccount(id);
			testResults = { ...testResults, [id]: res };
			if (res.ok) {
				toast.success('Account Reachable', `Latency: ${res.latency_ms}ms`);
			} else {
				toast.error('Test Failed', res.error || 'Account token is invalid');
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
		if (!confirm('Are you sure you want to remove this Kilo Code account?')) return;
		try {
			deletingId = id;
			await kilocodeState.deleteAccount(id);
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
				<Card.Title class="text-base font-semibold">Kilo Code Accounts</Card.Title>
				<Badge variant="outline" class="font-mono text-xs">
					{kilocodeState.activeAccountsCount} active / {kilocodeState.totalAccountsCount} total
				</Badge>
			</div>
			<Card.Description class="text-xs">
				Connected accounts used to execute Kilo Code OpenRouter gateway completions.
			</Card.Description>
		</div>

		<div class="flex items-center gap-2">
			<Button
				variant="outline"
				size="sm"
				class="gap-1.5 text-xs"
				onclick={() => kilocodeState.loadAccounts()}
				disabled={kilocodeState.accountsLoading}
			>
				<RefreshCwIcon class={`size-3.5 ${kilocodeState.accountsLoading ? 'animate-spin' : ''}`} />
				<span>Refresh</span>
			</Button>

			<Button
				size="sm"
				class="gap-1.5 text-xs bg-orange-600 hover:bg-orange-700 text-white"
				onclick={() => (dialogOpen = true)}
			>
				<PlusIcon class="size-3.5" />
				<span>Connect Account</span>
			</Button>
		</div>
	</Card.Header>

	<Card.Content class="p-0">
		{#if kilocodeState.accountsLoading && kilocodeState.accounts.length === 0}
			<div class="flex items-center justify-center py-12">
				<RefreshCwIcon class="size-6 animate-spin text-muted-foreground" />
			</div>
		{:else if kilocodeState.accounts.length === 0}
			<div class="flex flex-col items-center justify-center p-8 text-center sm:p-12">
				<div class="flex size-12 items-center justify-center rounded-2xl bg-orange-500/10 text-orange-500 mb-3">
					<UserIcon class="size-6" />
				</div>
				<h4 class="text-sm font-semibold text-foreground">No Kilo Code Accounts</h4>
				<p class="mt-1 max-w-sm text-xs text-muted-foreground">
					Connect a Kilo Code account via device authorization or API token to access Claude, Gemini, GPT, DeepSeek, and free models.
				</p>
				<Button
					size="sm"
					class="mt-4 gap-1.5 text-xs bg-orange-600 hover:bg-orange-700 text-white"
					onclick={() => (dialogOpen = true)}
				>
					<PlusIcon class="size-3.5" />
					<span>Connect First Account</span>
				</Button>
			</div>
		{:else}
			<Table.Root>
				<Table.Header>
					<Table.Row class="hover:bg-transparent">
						<Table.Head class="text-xs">Email / ID</Table.Head>
						<Table.Head class="text-xs">Organization</Table.Head>
						<Table.Head class="text-xs">Token</Table.Head>
						<Table.Head class="text-xs">Status</Table.Head>
						<Table.Head class="text-end text-xs">Actions</Table.Head>
					</Table.Row>
				</Table.Header>
				<Table.Body>
					{#each kilocodeState.accounts as acc (acc.id)}
						<Table.Row class="group">
							<Table.Cell class="py-3">
								<div class="flex items-center gap-2.5">
									<div
										class="flex size-7 shrink-0 items-center justify-center rounded-full bg-orange-500/10 font-mono text-xs font-semibold text-orange-500"
									>
										{acc.email ? acc.email.charAt(0).toUpperCase() : 'K'}
									</div>
									<div class="flex flex-col">
										<span class="text-xs font-medium text-foreground">{acc.email}</span>
										<span class="font-mono text-[10px] text-muted-foreground">{acc.id}</span>
									</div>
								</div>
							</Table.Cell>

							<Table.Cell class="py-3">
								{#if acc.orgId}
									<Badge variant="outline" class="font-mono text-[10px]">
										{acc.orgId}
									</Badge>
								{:else}
									<span class="text-xs text-muted-foreground">—</span>
								{/if}
							</Table.Cell>

							<Table.Cell class="py-3">
								<code class="rounded bg-muted px-1.5 py-0.5 font-mono text-[11px] text-muted-foreground">
									{acc.accessToken || '••••••••'}
								</code>
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
										title="Test token validity"
									>
										<PlayIcon class={`size-3.5 ${testingId === acc.id ? 'animate-spin' : ''}`} />
									</Button>

									<Button
										variant="ghost"
										size="icon-xs"
										class="text-muted-foreground hover:text-destructive"
										onclick={() => handleDeleteAccount(acc.id)}
										disabled={deletingId === acc.id}
										title="Remove account"
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
		await kilocodeState.loadAccounts();
	}}
	onClose={() => (dialogOpen = false)}
/>
