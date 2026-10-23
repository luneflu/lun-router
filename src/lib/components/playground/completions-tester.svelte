<script lang="ts">
	import * as Card from '$lib/components/ui/card';
	import * as Alert from '$lib/components/ui/alert';
	import * as Select from '$lib/components/ui/select';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Badge } from '$lib/components/ui/badge';
	import { Switch } from '$lib/components/ui/switch';
	import type { APIKey, ProviderModel } from '$lib/types';
	import { sendChatCompletion } from '$lib/api/completions';

	import BotIcon from '@lucide/svelte/icons/bot';
	import PlayIcon from '@lucide/svelte/icons/play';
	import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';
	import CopyIcon from '@lucide/svelte/icons/copy';
	import CheckIcon from '@lucide/svelte/icons/check';
	import AlertCircleIcon from '@lucide/svelte/icons/alert-circle';

	interface Props {
		keys: APIKey[];
		models?: ProviderModel[];
		selectedModel?: string;
	}

	let { keys, models, selectedModel }: Props = $props();

	const defaultFallbackModels = [
		'big-pickle',
		'ling-3.1-flash-free',
		'cat-jam-free',
		'mimo-v2.6-flash-free',
		'nemotron-3.5-lightning-free',
		'longcat-2.5-preview-free',
		'space-bunny-free',
		'muse-spark-1.2-contributor-free',
		'muse-spark-1.3-contributor-free'
	];

	let model = $state('big-pickle');
	let prompt = $state('Write a haiku about routing AI models.');
	let stream = $state(true);
	let selectedKeyId = $state('');
	let loading = $state(false);
	let responseText = $state('');
	let error = $state<string | null>(null);
	let latencyMs = $state<number | null>(null);
	let copied = $state(false);

	let modelOptions = $derived(
		models && models.length > 0
			? models.map((m) => ({ id: m.id, enabled: m.enabled }))
			: defaultFallbackModels.map((m) => ({ id: m, enabled: true }))
	);

	// Sync with selectedModel prop if passed
	$effect(() => {
		if (selectedModel) {
			model = selectedModel;
		}
	});

	// Auto-select first active key if none selected
	$effect(() => {
		if (!selectedKeyId && keys.length > 0) {
			const active = keys.find((k) => k.isActive);
			if (active) selectedKeyId = active.id;
		}
	});

	async function runTest() {
		const targetKey = keys.find((k) => k.id === selectedKeyId) || keys.find((k) => k.isActive);
		if (!targetKey) {
			error = 'Please create and activate an API key first.';
			return;
		}
		if (!prompt.trim()) {
			error = 'Please enter a test prompt.';
			return;
		}

		try {
			loading = true;
			error = null;
			responseText = '';
			const startTime = performance.now();

			await sendChatCompletion(
				{
					model,
					messages: [{ role: 'user', content: prompt.trim() }],
					stream
				},
				targetKey.key,
				{
					onDelta: (delta) => {
						responseText += delta;
					}
				}
			).then((full) => {
				if (!stream) responseText = full;
			});

			latencyMs = Math.round(performance.now() - startTime);
		} catch (e: unknown) {
			error = e instanceof Error ? e.message : 'Completion test failed';
		} finally {
			loading = false;
		}
	}

	async function copyResponse() {
		if (!responseText) return;
		try {
			await navigator.clipboard.writeText(responseText);
			copied = true;
			setTimeout(() => {
				copied = false;
			}, 2000);
		} catch (e: unknown) {
			console.error('Copy failed', e);
		}
	}

	function maskKey(key: string): string {
		if (key.length <= 14) return '••••••••••••';
		return `${key.slice(0, 7)}••••••••${key.slice(-4)}`;
	}
</script>

<Card.Root class="border-primary/20 bg-card">
	<Card.Header
		class="flex flex-col justify-between gap-2 border-b border-border pb-3 sm:flex-row sm:items-center"
	>
		<div class="flex flex-col gap-1">
			<div class="flex items-center gap-2">
				<BotIcon class="size-4 text-primary" />
				<Card.Title class="text-base font-semibold">OpenCode Completions Tester</Card.Title>
				<Badge variant="secondary" class="font-mono text-[10px]">/v1/chat/completions</Badge>
			</div>
			<Card.Description class="text-xs">
				Test completions against upstream OpenCode with automatic session headers and
				fingerprinting.
			</Card.Description>
		</div>
		{#if latencyMs !== null}
			<Badge variant="outline" class="w-fit font-mono text-xs">
				{latencyMs}ms
			</Badge>
		{/if}
	</Card.Header>

	<Card.Content class="flex flex-col gap-4 p-4 sm:p-5">
		<div class="grid grid-cols-1 gap-3 sm:grid-cols-3">
			<!-- Model Selection -->
			<div class="flex flex-col gap-1.5">
				<span class="text-xs font-medium text-muted-foreground">Model</span>
				<Select.Root type="single" bind:value={model}>
					<Select.Trigger class="h-9 w-full font-mono text-xs">
						<Select.Value placeholder="Select model" />
					</Select.Trigger>
					<Select.Content class="max-h-72">
						<Select.Group>
							{#each modelOptions as m (m.id)}
								<Select.Item value={m.id} disabled={!m.enabled} label={m.id}>
									<span class="font-mono text-xs">{m.id}</span>
									{#if !m.enabled}
										<span class="text-[10px] text-muted-foreground">(Disabled)</span>
									{/if}
								</Select.Item>
							{/each}
						</Select.Group>
					</Select.Content>
				</Select.Root>
			</div>

			<!-- Key Selection -->
			<div class="flex flex-col gap-1.5">
				<span class="text-xs font-medium text-muted-foreground">Authorized API Key</span>
				<Select.Root type="single" bind:value={selectedKeyId}>
					<Select.Trigger class="h-9 w-full text-xs">
						<Select.Value placeholder="Select API key" />
					</Select.Trigger>
					<Select.Content class="max-h-72">
						<Select.Group>
							{#if keys.length === 0}
								<Select.Item value="" disabled label="No keys created yet">
									No keys created yet
								</Select.Item>
							{:else}
								{#each keys as k (k.id)}
									<Select.Item
										value={k.id}
										disabled={!k.isActive}
										label={`${k.name} (${maskKey(k.key)})`}
									>
										<span>{k.name} ({maskKey(k.key)})</span>
										{#if !k.isActive}
											<span class="text-[10px] text-muted-foreground">[disabled]</span>
										{/if}
									</Select.Item>
								{/each}
							{/if}
						</Select.Group>
					</Select.Content>
				</Select.Root>
			</div>

			<!-- Streaming Toggle -->
			<div class="flex flex-col justify-end gap-1.5">
				<div
					class="flex h-9 items-center justify-between rounded-md border border-input bg-background px-3"
				>
					<span class="text-xs font-medium text-muted-foreground">Stream SSE chunks</span>
					<Switch bind:checked={stream} size="sm" aria-label="Toggle streaming" />
				</div>
			</div>
		</div>

		<!-- Prompt Input & Run Button -->
		<div class="flex flex-col gap-1.5">
			<label for="test-prompt" class="text-xs font-medium text-muted-foreground">Prompt</label>
			<div class="flex flex-col gap-2 sm:flex-row">
				<Input
					id="test-prompt"
					bind:value={prompt}
					placeholder="Ask anything..."
					class="flex-1 text-xs"
					disabled={loading}
					onkeydown={(e) => {
						if (e.key === 'Enter') runTest();
					}}
				/>
				<Button
					variant="default"
					size="sm"
					class="shrink-0"
					disabled={loading || keys.length === 0}
					onclick={runTest}
				>
					{#if loading}
						<RefreshCwIcon data-icon="inline-start" class="animate-spin" />
						Generating...
					{:else}
						<PlayIcon data-icon="inline-start" />
						Send Prompt
					{/if}
				</Button>
			</div>
		</div>

		{#if error}
			<Alert.Root variant="destructive" class="px-3 py-2.5 text-xs">
				<AlertCircleIcon class="size-4" />
				<Alert.Title class="text-xs">Completion Error</Alert.Title>
				<Alert.Description class="text-xs">{error}</Alert.Description>
			</Alert.Root>
		{/if}

		{#if responseText || loading}
			<div class="mt-1 flex flex-col gap-1.5">
				<div class="flex items-center justify-between">
					<span class="text-xs font-medium text-muted-foreground">OpenCode Response:</span>
					{#if responseText}
						<Button
							variant="ghost"
							size="icon-sm"
							class="size-6 text-muted-foreground hover:text-foreground"
							onclick={copyResponse}
							title="Copy Response"
						>
							{#if copied}
								<CheckIcon class="size-3 text-emerald-500" />
							{:else}
								<CopyIcon class="size-3" />
							{/if}
						</Button>
					{/if}
				</div>
				<div
					class="min-h-[72px] rounded-lg border border-border bg-muted/60 p-3.5 font-mono text-xs leading-relaxed whitespace-pre-wrap"
				>
					{responseText}{#if loading}<span
							class="ms-1 inline-block size-1.5 animate-ping rounded-full bg-primary"
						></span>{/if}
				</div>
			</div>
		{/if}
	</Card.Content>
</Card.Root>
