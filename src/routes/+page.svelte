<script lang="ts">
	import { onMount } from 'svelte';
	import * as Card from '$lib/components/ui/card';
	import * as Dialog from '$lib/components/ui/dialog';
	import * as Table from '$lib/components/ui/table';
	import * as Alert from '$lib/components/ui/alert';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Badge } from '$lib/components/ui/badge';
	import { Switch } from '$lib/components/ui/switch';
	import { cn } from '$lib/utils';

	import KeyRoundIcon from '@lucide/svelte/icons/key-round';
	import PlusIcon from '@lucide/svelte/icons/plus';
	import CopyIcon from '@lucide/svelte/icons/copy';
	import CheckIcon from '@lucide/svelte/icons/check';
	import EyeIcon from '@lucide/svelte/icons/eye';
	import EyeOffIcon from '@lucide/svelte/icons/eye-off';
	import Trash2Icon from '@lucide/svelte/icons/trash-2';
	import SunIcon from '@lucide/svelte/icons/sun';
	import MoonIcon from '@lucide/svelte/icons/moon';
	import SearchIcon from '@lucide/svelte/icons/search';
	import AlertCircleIcon from '@lucide/svelte/icons/alert-circle';
	import ServerIcon from '@lucide/svelte/icons/server';
	import CheckCircle2Icon from '@lucide/svelte/icons/check-circle-2';
	import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';

	interface APIKey {
		id: string;
		key: string;
		name: string;
		machineId?: string;
		isActive: boolean;
		createdAt: string;
	}

	let keys = $state<APIKey[]>([]);
	let loading = $state(true);
	let error = $state<string | null>(null);

	// Theme state
	let isDark = $state(true);

	// Modal states
	let isCreateOpen = $state(false);
	let createName = $state('');
	let createMode = $state<'auto' | 'custom'>('auto');
	let customKey = $state('');
	let creating = $state(false);
	let modalError = $state<string | null>(null);

	// Key created modal
	let justCreatedKey = $state<APIKey | null>(null);
	let isSuccessOpen = $state(false);

	// Interaction states
	let visibleKeys = $state<Record<string, boolean>>({});
	let copiedId = $state<string | null>(null);
	let searchQuery = $state('');

	const filteredKeys = $derived(
		keys.filter(
			(k) =>
				k.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
				k.key.toLowerCase().includes(searchQuery.toLowerCase())
		)
	);

	const activeCount = $derived(keys.filter((k) => k.isActive).length);
	const primaryMachineId = $derived(keys.find((k) => k.machineId)?.machineId || 'local-host');

	async function fetchKeys() {
		try {
			loading = true;
			error = null;
			const res = await fetch('/api/keys');
			if (!res.ok) {
				throw new Error(`Failed to fetch keys (${res.status} ${res.statusText})`);
			}
			const data = await res.json();
			keys = data.keys || [];
		} catch (e: unknown) {
			error = e instanceof Error ? e.message : 'Go backend unreachable on :8080';
		} finally {
			loading = false;
		}
	}

	async function handleCreateKey(e: SubmitEvent) {
		e.preventDefault();
		if (!createName.trim()) {
			modalError = 'Please provide a name for the key';
			return;
		}
		if (createMode === 'custom' && !customKey.trim()) {
			modalError = 'Custom key string cannot be empty';
			return;
		}

		try {
			creating = true;
			modalError = null;
			const payload: { name: string; key?: string } = {
				name: createName.trim()
			};
			if (createMode === 'custom' && customKey.trim()) {
				payload.key = customKey.trim();
			}

			const res = await fetch('/api/keys', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify(payload)
			});

			if (!res.ok) {
				const errData = await res.json().catch(() => ({}));
				throw new Error(errData.error || 'Failed to create key');
			}

			const newKey: APIKey = await res.json();
			justCreatedKey = newKey;
			isSuccessOpen = true;
			isCreateOpen = false;
			createName = '';
			customKey = '';
			createMode = 'auto';
			await fetchKeys();
		} catch (e: unknown) {
			modalError = e instanceof Error ? e.message : 'Error creating key';
		} finally {
			creating = false;
		}
	}

	async function handleToggleActive(key: APIKey, checked: boolean) {
		try {
			const res = await fetch(`/api/keys/${key.id}`, {
				method: 'PUT',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ isActive: checked })
			});
			if (res.ok) {
				keys = keys.map((k) => (k.id === key.id ? { ...k, isActive: checked } : k));
			}
		} catch (e: unknown) {
			console.error('Failed to toggle key status', e);
		}
	}

	async function handleDeleteKey(id: string) {
		if (!confirm('Are you sure you want to delete this API key? This action cannot be undone.')) {
			return;
		}
		try {
			const res = await fetch(`/api/keys/${id}`, { method: 'DELETE' });
			if (res.ok) {
				keys = keys.filter((k) => k.id !== id);
			}
		} catch (e: unknown) {
			console.error('Failed to delete key', e);
		}
	}

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

	function toggleTheme() {
		isDark = !isDark;
		applyTheme(isDark);
	}

	function applyTheme(dark: boolean) {
		const root = document.documentElement;
		if (dark) {
			root.classList.add('dark');
		} else {
			root.classList.remove('dark');
		}
	}

	onMount(() => {
		isDark =
			document.documentElement.classList.contains('dark') ||
			window.matchMedia('(prefers-color-scheme: dark)').matches;
		applyTheme(isDark);
		fetchKeys();
	});
</script>

<div class="flex min-h-screen flex-col bg-background text-foreground">
	<!-- Navbar / Header -->
	<header
		class="sticky top-0 z-30 border-b border-border bg-card/75 px-4 py-3 backdrop-blur-md sm:px-8"
	>
		<div class="mx-auto flex max-w-6xl items-center justify-between">
			<div class="flex items-center gap-3">
				<div
					class="flex size-9 items-center justify-center rounded-lg border border-primary/20 bg-primary/10 text-primary"
				>
					<KeyRoundIcon class="size-4" />
				</div>
				<div class="flex flex-col">
					<div class="flex items-center gap-2">
						<span class="text-base font-semibold tracking-tight">Lun Router</span>
						<Badge variant="secondary" class="text-[10px] font-medium">9router Go</Badge>
					</div>
					<span class="text-xs text-muted-foreground">API Key & Endpoint Management</span>
				</div>
			</div>

			<div class="flex items-center gap-2 sm:gap-3">
				<!-- Backend status indicator -->
				<div class="hidden items-center gap-2 rounded-full bg-muted px-2.5 py-1 text-xs sm:flex">
					<span
						class={cn(
							'size-2 rounded-full',
							error ? 'animate-pulse bg-destructive' : 'bg-emerald-500'
						)}
					></span>
					<span class="text-muted-foreground">Backend:</span>
					<span
						class={cn(
							'font-mono text-[11px]',
							error ? 'font-medium text-destructive' : 'text-foreground'
						)}
					>
						{error ? 'offline' : 'Go :8080'}
					</span>
				</div>

				<!-- Dark / Light theme toggle -->
				<Button
					variant="ghost"
					size="icon-sm"
					onclick={toggleTheme}
					aria-label={isDark ? 'Switch to light theme' : 'Switch to dark theme'}
					title={isDark ? 'Switch to light theme' : 'Switch to dark theme'}
				>
					{#if isDark}
						<SunIcon class="size-4" />
					{:else}
						<MoonIcon class="size-4" />
					{/if}
				</Button>

				<!-- Create Key Button -->
				<Button
					variant="default"
					size="sm"
					onclick={() => {
						modalError = null;
						isCreateOpen = true;
					}}
				>
					<PlusIcon data-icon="inline-start" />
					Create Key
				</Button>
			</div>
		</div>
	</header>

	<!-- Main Body Content -->
	<main class="mx-auto flex w-full max-w-6xl flex-1 flex-col gap-6 px-4 py-6 sm:px-8">
		{#if error}
			<Alert.Root variant="destructive">
				<AlertCircleIcon class="size-4" />
				<Alert.Title>Go Backend Unreachable</Alert.Title>
				<Alert.Description class="flex items-center justify-between gap-4">
					<span>{error}. Please run <code>go run .</code> from the project root.</span>
					<Button variant="outline" size="sm" onclick={fetchKeys}>
						<RefreshCwIcon data-icon="inline-start" />
						Retry
					</Button>
				</Alert.Description>
			</Alert.Root>
		{/if}

		<!-- Stats Overview Cards -->
		<div class="grid grid-cols-1 gap-4 sm:grid-cols-3">
			<Card.Root>
				<Card.Header class="flex flex-row items-center justify-between pb-2">
					<Card.Title class="text-xs font-medium text-muted-foreground">Total API Keys</Card.Title>
					<div
						class="flex size-8 items-center justify-center rounded-md bg-muted text-muted-foreground"
					>
						<KeyRoundIcon class="size-4" />
					</div>
				</Card.Header>
				<Card.Content>
					<div class="text-2xl font-bold">{keys.length}</div>
					<p class="mt-1 text-xs text-muted-foreground">Managed router credentials</p>
				</Card.Content>
			</Card.Root>

			<Card.Root>
				<Card.Header class="flex flex-row items-center justify-between pb-2">
					<Card.Title class="text-xs font-medium text-muted-foreground">Active Keys</Card.Title>
					<div
						class="flex size-8 items-center justify-center rounded-md bg-emerald-500/10 text-emerald-500"
					>
						<CheckCircle2Icon class="size-4" />
					</div>
				</Card.Header>
				<Card.Content>
					<div class="text-2xl font-bold text-emerald-500">{activeCount}</div>
					<p class="mt-1 text-xs text-muted-foreground">Accepting incoming traffic</p>
				</Card.Content>
			</Card.Root>

			<Card.Root>
				<Card.Header class="flex flex-row items-center justify-between pb-2">
					<Card.Title class="text-xs font-medium text-muted-foreground"
						>Router Machine ID</Card.Title
					>
					<div
						class="flex size-8 items-center justify-center rounded-md bg-muted text-muted-foreground"
					>
						<ServerIcon class="size-4" />
					</div>
				</Card.Header>
				<Card.Content>
					<div class="truncate font-mono text-sm" title={primaryMachineId}>{primaryMachineId}</div>
					<p class="mt-1 text-xs text-muted-foreground">Persistent hardware fingerprint</p>
				</Card.Content>
			</Card.Root>
		</div>

		<!-- Main Card with API Keys Table -->
		<Card.Root class="flex flex-col gap-0 overflow-hidden p-0">
			<Card.Header
				class="flex flex-col justify-between gap-4 border-b border-border p-4 sm:flex-row sm:items-center sm:p-5"
			>
				<div class="flex flex-col gap-1">
					<div class="flex items-center gap-2">
						<Card.Title class="text-base font-semibold">API Keys</Card.Title>
						<Badge variant="outline" class="font-mono text-xs">{filteredKeys.length}</Badge>
					</div>
					<Card.Description class="text-xs">
						Keys authorize requests to upstream AI models. Supports auto-generated CRC keys or
						custom keys.
					</Card.Description>
				</div>

				<div class="flex w-full items-center gap-2 sm:w-auto">
					<div class="relative w-full sm:w-64">
						<SearchIcon
							class="pointer-events-none absolute start-2.5 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground"
						/>
						<Input
							type="search"
							bind:value={searchQuery}
							placeholder="Search by name or key..."
							class="h-8 ps-8 text-xs"
						/>
					</div>

					<Button
						variant="default"
						size="sm"
						class="shrink-0"
						onclick={() => {
							modalError = null;
							isCreateOpen = true;
						}}
					>
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
						{:else if filteredKeys.length === 0}
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
											<Button
												variant="outline"
												size="sm"
												class="mt-2"
												onclick={() => (isCreateOpen = true)}
											>
												<PlusIcon data-icon="inline-start" />
												Create First Key
											</Button>
										{/if}
									</div>
								</Table.Cell>
							</Table.Row>
						{:else}
							{#each filteredKeys as key (key.id)}
								<Table.Row class={cn(!key.isActive && 'opacity-60')}>
									<!-- Name Column -->
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

									<!-- Key Column with Reveal & Copy -->
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

									<!-- Status Column with Badge & Switch -->
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
												onCheckedChange={(checked) => handleToggleActive(key, checked)}
												aria-label={`Toggle active state for ${key.name}`}
												size="sm"
											/>
										</div>
									</Table.Cell>

									<!-- Created Date Column -->
									<Table.Cell class="text-xs text-muted-foreground">
										{formatDate(key.createdAt)}
									</Table.Cell>

									<!-- Actions Column -->
									<Table.Cell class="text-end">
										<Button
											variant="ghost"
											size="icon-sm"
											class="text-destructive hover:bg-destructive/10"
											onclick={() => handleDeleteKey(key.id)}
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
				<span>Showing {filteredKeys.length} of {keys.length} API keys</span>
				<span class="font-mono text-[11px]">HMAC-SHA256 CRC8 enabled</span>
			</Card.Footer>
		</Card.Root>

		<!-- Router usage info card -->
		<Card.Root class="border-dashed bg-muted/30">
			<Card.Content
				class="flex flex-col justify-between gap-3 p-4 text-xs sm:flex-row sm:items-center"
			>
				<div class="flex flex-col gap-0.5">
					<span class="font-medium text-foreground">API Authentication Header</span>
					<span class="text-muted-foreground"
						>Send your API key in the standard HTTP Authorization header:</span
					>
				</div>
				<code
					class="rounded border border-border bg-background px-2.5 py-1 font-mono text-[11px] text-foreground select-all"
				>
					Authorization: Bearer sk-...
				</code>
			</Card.Content>
		</Card.Root>
	</main>
</div>

<!-- Dialog: Create API Key -->
<Dialog.Root bind:open={isCreateOpen}>
	<Dialog.Content class="sm:max-w-md">
		<Dialog.Header>
			<Dialog.Title class="flex items-center gap-2 text-base font-semibold">
				<KeyRoundIcon class="size-4 text-primary" />
				Create API Key
			</Dialog.Title>
			<Dialog.Description class="text-xs">
				Generate a new router key or assign your custom key string.
			</Dialog.Description>
		</Dialog.Header>

		<form onsubmit={handleCreateKey} class="mt-2 flex flex-col gap-4">
			{#if modalError}
				<Alert.Root variant="destructive" class="px-3 py-2 text-xs">
					<AlertCircleIcon class="size-4" />
					<Alert.Description>{modalError}</Alert.Description>
				</Alert.Root>
			{/if}

			<!-- Name Field -->
			<div class="flex flex-col gap-1.5">
				<label for="create-key-name" class="text-xs font-medium text-foreground">
					Key Name / Label <span class="text-destructive">*</span>
				</label>
				<Input
					id="create-key-name"
					type="text"
					bind:value={createName}
					placeholder="e.g. Production Client, Cursor IDE, Claude CLI"
					required
				/>
				<span class="text-[11px] text-muted-foreground">Human-friendly name to identify usage.</span
				>
			</div>

			<!-- Strategy Selection -->
			<div class="flex flex-col gap-1.5">
				<span class="text-xs font-medium text-foreground">Key Generation Strategy</span>
				<div class="grid grid-cols-2 gap-1 rounded-lg border border-border bg-muted p-1">
					<button
						type="button"
						class={cn(
							'rounded-md px-3 py-1.5 text-xs font-medium transition-all',
							createMode === 'auto'
								? 'bg-background text-foreground shadow-xs'
								: 'text-muted-foreground hover:text-foreground'
						)}
						onclick={() => (createMode = 'auto')}
					>
						Auto-generate
					</button>
					<button
						type="button"
						class={cn(
							'rounded-md px-3 py-1.5 text-xs font-medium transition-all',
							createMode === 'custom'
								? 'bg-background text-foreground shadow-xs'
								: 'text-muted-foreground hover:text-foreground'
						)}
						onclick={() => (createMode = 'custom')}
					>
						Custom Key
					</button>
				</div>
			</div>

			{#if createMode === 'auto'}
				<Alert.Root class="border-border bg-muted/40 px-3 py-2.5 text-xs">
					<ServerIcon class="size-4" />
					<Alert.Title class="text-xs font-medium">9router Format Specification</Alert.Title>
					<Alert.Description class="mt-0.5 font-mono text-[11px] text-muted-foreground">
						sk-{primaryMachineId.slice(0, 6)}...-[keyId]-[crc8]
					</Alert.Description>
				</Alert.Root>
			{:else}
				<div class="flex flex-col gap-1.5">
					<label for="create-custom-key" class="text-xs font-medium text-foreground">
						Custom API Key Value <span class="text-destructive">*</span>
					</label>
					<Input
						id="create-custom-key"
						type="text"
						bind:value={customKey}
						placeholder="sk-my-custom-endpoint-key"
						class="font-mono text-xs"
						required
					/>
					<span class="text-[11px] text-muted-foreground">
						Enter any custom token or pre-shared key you want to use.
					</span>
				</div>
			{/if}

			<Dialog.Footer class="gap-2 pt-2">
				<Button type="button" variant="ghost" size="sm" onclick={() => (isCreateOpen = false)}>
					Cancel
				</Button>
				<Button type="submit" variant="default" size="sm" disabled={creating}>
					{creating ? 'Creating...' : 'Create API Key'}
				</Button>
			</Dialog.Footer>
		</form>
	</Dialog.Content>
</Dialog.Root>

<!-- Dialog: Key Created Success -->
<Dialog.Root bind:open={isSuccessOpen}>
	<Dialog.Content class="sm:max-w-md">
		<Dialog.Header>
			<Dialog.Title class="flex items-center gap-2 text-base font-semibold text-emerald-500">
				<CheckCircle2Icon class="size-4" />
				API Key Created
			</Dialog.Title>
			<Dialog.Description class="text-xs">
				Your key for <strong>{justCreatedKey?.name}</strong> is ready to use.
			</Dialog.Description>
		</Dialog.Header>

		<div class="mt-2 flex flex-col gap-4">
			<Alert.Root class="border-amber-500/20 bg-amber-500/10 px-3 py-2.5 text-xs text-amber-500">
				<AlertCircleIcon class="size-4" />
				<Alert.Title class="text-xs font-semibold">Copy your key now</Alert.Title>
				<Alert.Description class="text-[11px] text-amber-600 dark:text-amber-400">
					This key is shown in full only once. It will be masked in the dashboard.
				</Alert.Description>
			</Alert.Root>

			<div class="flex flex-col gap-1.5">
				<span class="text-xs font-medium text-foreground">Generated Key Value</span>
				<div class="flex items-center gap-2">
					<Input
						type="text"
						readonly
						value={justCreatedKey?.key || ''}
						class="bg-muted/60 font-mono text-xs select-all"
					/>
					<Button
						variant="secondary"
						size="sm"
						class="shrink-0"
						onclick={() => justCreatedKey && copyToClipboard(justCreatedKey.key, 'created_success')}
					>
						{#if copiedId === 'created_success'}
							<CheckIcon data-icon="inline-start" class="text-emerald-500" />
							Copied!
						{:else}
							<CopyIcon data-icon="inline-start" />
							Copy
						{/if}
					</Button>
				</div>
			</div>

			<Dialog.Footer class="pt-2">
				<Button variant="default" size="sm" onclick={() => (isSuccessOpen = false)}>Done</Button>
			</Dialog.Footer>
		</div>
	</Dialog.Content>
</Dialog.Root>
