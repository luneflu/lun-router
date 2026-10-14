<script lang="ts">
	import * as Card from '$lib/components/ui/card';
	import * as Table from '$lib/components/ui/table';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Badge } from '$lib/components/ui/badge';
	import { Switch } from '$lib/components/ui/switch';
	import { cn } from '$lib/utils';
	import type { APIKey } from '$lib/types';

	import KeyRoundIcon from '@lucide/svelte/icons/key-round';
	import PlusIcon from '@lucide/svelte/icons/plus';
	import CopyIcon from '@lucide/svelte/icons/copy';
	import CheckIcon from '@lucide/svelte/icons/check';
	import EyeIcon from '@lucide/svelte/icons/eye';
	import EyeOffIcon from '@lucide/svelte/icons/eye-off';
	import Trash2Icon from '@lucide/svelte/icons/trash-2';
	import SearchIcon from '@lucide/svelte/icons/search';

	interface Props {
		keys: APIKey[];
		totalCount: number;
		loading: boolean;
		searchQuery: string;
		onSearchChange: (query: string) => void;
		onCreateClick: () => void;
		onToggleActive: (key: APIKey, checked: boolean) => void;
		onDeleteKey: (id: string) => void;
	}

	let {
		keys,
		totalCount,
		loading,
		searchQuery,
		onSearchChange,
		onCreateClick,
		onToggleActive,
		onDeleteKey
	}: Props = $props();

	let visibleKeys = $state<Record<string, boolean>>({});
	let copiedId = $state<string | null>(null);

	function toggleKeyVisibility(id: string) {
		visibleKeys = { ...visibleKeys, [id]: !visibleKeys[id] };
	}

	async function copyToClipboard(text: string, id: string) {
		try {
			await navigator.clipboard.writeText(text);
			copiedId = id;
			setTimeout(() => {
				if (copiedId === id) copiedId = null;
			}, 2000);
		} catch (e: unknown) {
			console.error('Copy failed', e);
		}
	}

	function maskKey(key: string): string {
		if (key.length <= 14) return '••••••••••••';
		return `${key.slice(0, 7)}••••••••${key.slice(-4)}`;
	}

	function formatDate(iso: string): string {
		try {
			return new Date(iso).toLocaleDateString(undefined, {
				year: 'numeric',
				month: 'short',
				day: 'numeric'
			});
		} catch {
			return iso;
		}
	}
</script>

<Card.Root class="flex flex-col gap-0 overflow-hidden p-0">
	<Card.Header
		class="flex flex-col justify-between gap-4 border-b border-border p-4 sm:flex-row sm:items-center sm:p-5"
	>
		<div class="flex flex-col gap-1">
			<div class="flex items-center gap-2">
				<Card.Title class="text-base font-semibold">API Keys</Card.Title>
				<Badge variant="outline" class="font-mono text-xs">{keys.length}</Badge>
			</div>
			<Card.Description class="text-xs">
				Manage router keys. Keys authenticate endpoints like <code>/v1/chat/completions</code> and proxy
				to OpenCode Free.
			</Card.Description>
		</div>

		<div class="flex w-full items-center gap-2 sm:w-auto">
			<div class="relative w-full sm:w-64">
				<SearchIcon
					class="pointer-events-none absolute start-2.5 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground"
				/>
				<Input
					type="search"
					value={searchQuery}
					oninput={(e) => onSearchChange((e.target as HTMLInputElement).value)}
					placeholder="Search by name or key..."
					class="h-8 ps-8 text-xs"
				/>
			</div>

			<Button variant="default" size="sm" class="shrink-0" onclick={onCreateClick}>
				<PlusIcon data-icon="inline-start" />
				Create
			</Button>
		</div>
	</Card.Header>

	<Card.Content class="p-0">
		<Table.Root>
			<Table.Header>
				<Table.Row class="hover:bg-transparent">
					<Table.Head class="w-[200px]">NAME</Table.Head>
					<Table.Head>API KEY</Table.Head>
					<Table.Head class="w-[140px]">STATUS</Table.Head>
					<Table.Head class="w-[140px]">CREATED</Table.Head>
					<Table.Head class="w-[80px] text-end">ACTIONS</Table.Head>
				</Table.Row>
			</Table.Header>
			<Table.Body>
				{#if loading}
					<Table.Row>
						<Table.Cell colspan={5} class="h-32 text-center text-muted-foreground">
							<div class="flex flex-col items-center justify-center gap-2">
								<div
									class="size-4 animate-spin rounded-full border-2 border-primary border-t-transparent"
								></div>
								<span class="text-xs">Loading keys...</span>
							</div>
						</Table.Cell>
					</Table.Row>
				{:else if keys.length === 0}
					<Table.Row>
						<Table.Cell colspan={5} class="h-40 text-center">
							<div class="flex flex-col items-center justify-center gap-2">
								<div
									class="flex size-10 items-center justify-center rounded-full bg-muted text-muted-foreground"
								>
									<KeyRoundIcon class="size-5" />
								</div>
								<p class="text-sm font-medium">No API keys found</p>
								<p class="text-xs text-muted-foreground">
									{searchQuery
										? 'No keys match your search query.'
										: 'Create your first API key to start authenticating.'}
								</p>
								{#if !searchQuery}
									<Button variant="outline" size="sm" class="mt-2" onclick={onCreateClick}>
										<PlusIcon data-icon="inline-start" />
										Create First Key
									</Button>
								{/if}
							</div>
						</Table.Cell>
					</Table.Row>
				{:else}
					{#each keys as key (key.id)}
						<Table.Row class={cn(!key.isActive && 'opacity-60')}>
							<Table.Cell class="font-medium">
								<div class="flex flex-col gap-0.5">
									<span class="text-sm font-medium text-foreground">{key.name}</span>
									{#if key.machineId}
										<span class="font-mono text-[10px] text-muted-foreground">
											mid:{key.machineId.slice(0, 6)}
										</span>
									{/if}
								</div>
							</Table.Cell>

							<Table.Cell>
								<div
									class="inline-flex items-center gap-1.5 rounded-md border border-border bg-muted/70 px-2 py-1"
								>
									<code class="font-mono text-xs select-all">
										{visibleKeys[key.id] ? key.key : maskKey(key.key)}
									</code>

									<Button
										variant="ghost"
										size="icon-sm"
										class="size-6 text-muted-foreground hover:text-foreground"
										onclick={() => toggleKeyVisibility(key.id)}
										title={visibleKeys[key.id] ? 'Hide Key' : 'Reveal Key'}
										aria-label={visibleKeys[key.id] ? 'Hide Key' : 'Reveal Key'}
									>
										{#if visibleKeys[key.id]}
											<EyeOffIcon class="size-3.5" />
										{:else}
											<EyeIcon class="size-3.5" />
										{/if}
									</Button>

									<Button
										variant="ghost"
										size="icon-sm"
										class="size-6 text-muted-foreground hover:text-foreground"
										onclick={() => copyToClipboard(key.key, key.id)}
										title="Copy Key"
										aria-label="Copy Key"
									>
										{#if copiedId === key.id}
											<CheckIcon class="size-3.5 text-emerald-500" />
										{:else}
											<CopyIcon class="size-3.5" />
										{/if}
									</Button>
								</div>
							</Table.Cell>

							<Table.Cell>
								<div class="flex items-center gap-2">
									{#if key.isActive}
										<Badge
											variant="default"
											class="border-emerald-500/20 bg-emerald-500/10 text-[10px] text-emerald-500 hover:bg-emerald-500/20"
										>
											Active
										</Badge>
									{:else}
										<Badge variant="secondary" class="text-[10px]">Disabled</Badge>
									{/if}
									<Switch
										checked={key.isActive}
										onCheckedChange={(checked) => onToggleActive(key, checked)}
										aria-label={`Toggle active state for ${key.name}`}
										size="sm"
									/>
								</div>
							</Table.Cell>

							<Table.Cell class="text-xs text-muted-foreground">
								{formatDate(key.createdAt)}
							</Table.Cell>

							<Table.Cell class="text-end">
								<Button
									variant="ghost"
									size="icon-sm"
									class="text-destructive hover:bg-destructive/10"
									onclick={() => onDeleteKey(key.id)}
									title={`Delete ${key.name}`}
									aria-label={`Delete ${key.name}`}
								>
									<Trash2Icon class="size-3.5" />
								</Button>
							</Table.Cell>
						</Table.Row>
					{/each}
				{/if}
			</Table.Body>
		</Table.Root>
	</Card.Content>

	<Card.Footer
		class="flex items-center justify-between border-t border-border bg-muted/20 p-3 text-xs text-muted-foreground"
	>
		<span>Showing {keys.length} of {totalCount} API keys</span>
		<span class="font-mono text-[11px]">HMAC-SHA256 CRC8 enabled</span>
	</Card.Footer>
</Card.Root>
