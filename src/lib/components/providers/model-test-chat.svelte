<script lang="ts">
	import * as Card from '$lib/components/ui/card';
	import * as Alert from '$lib/components/ui/alert';
	import * as Select from '$lib/components/ui/select';
	import { Button } from '$lib/components/ui/button';
	import { Badge } from '$lib/components/ui/badge';
	import { cn } from '$lib/utils';
	import type { ProviderModel, ChatMessage } from '$lib/types';
	import { sendChatCompletion } from '$lib/api/completions';
	import { keysState } from '$lib/state/keys.svelte';

	import MessageSquareIcon from '@lucide/svelte/icons/message-square';
	import SendIcon from '@lucide/svelte/icons/send';
	import Trash2Icon from '@lucide/svelte/icons/trash-2';
	import BotIcon from '@lucide/svelte/icons/bot';
	import UserIcon from '@lucide/svelte/icons/user';
	import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';
	import SquareIcon from '@lucide/svelte/icons/square';
	import CopyIcon from '@lucide/svelte/icons/copy';
	import CheckIcon from '@lucide/svelte/icons/check';
	import SparklesIcon from '@lucide/svelte/icons/sparkles';
	import ChevronDownIcon from '@lucide/svelte/icons/chevron-down';
	import ChevronUpIcon from '@lucide/svelte/icons/chevron-up';
	import AlertCircleIcon from '@lucide/svelte/icons/alert-circle';

	interface TestMessage {
		id: string;
		role: 'user' | 'assistant';
		content: string;
		latencyMs?: number;
		timestamp: Date;
	}

	interface Props {
		models: ProviderModel[];
		selectedModelId?: string;
		onSelectModel?: (modelId: string) => void;
	}

	let { models, selectedModelId = $bindable('big-pickle'), onSelectModel }: Props = $props();

	let messages = $state<TestMessage[]>([]);
	let inputText = $state('');
	let isGenerating = $state(false);
	let error = $state<string | null>(null);
	let copiedId = $state<string | null>(null);
	let isCollapsed = $state(false);
	let chatContainer = $state<HTMLDivElement | null>(null);
	let abortController: AbortController | null = null;

	const fallbackModels = [
		'big-pickle',
		'ling-3.1-flash-free',
		'cat-jam-free',
		'mimo-v2.6-flash-free',
		'nemotron-3.5-lightning-free'
	];

	let modelList = $derived(
		models && models.length > 0
			? models.map((m) => ({ id: m.id, enabled: m.enabled }))
			: fallbackModels.map((m) => ({ id: m, enabled: true }))
	);

	let currentModel = $derived(models?.find((m) => m.id === selectedModelId));
	let thinkingLevels = $derived(currentModel?.thinking_levels || []);
	let selectedThinkingLevel = $state<string>('auto');

	// Select first enabled model if current selected model is empty or invalid
	$effect(() => {
		if (modelList.length > 0) {
			const exists = modelList.some((m) => m.id === selectedModelId);
			if (!exists) {
				const firstEnabled = modelList.find((m) => m.enabled) || modelList[0];
				selectedModelId = firstEnabled.id;
			}
		}
	});

	function handleModelChange(val: string) {
		selectedModelId = val;
		onSelectModel?.(val);
	}

	function scrollToBottom() {
		if (chatContainer) {
			chatContainer.scrollTop = chatContainer.scrollHeight;
		}
	}

	async function handleSend(customPrompt?: string) {
		const text = (customPrompt ?? inputText).trim();
		if (!text || isGenerating) return;

		error = null;
		inputText = '';

		const userMsg: TestMessage = {
			id: crypto.randomUUID(),
			role: 'user',
			content: text,
			timestamp: new Date()
		};

		const assistantMsgId = crypto.randomUUID();
		const assistantMsg: TestMessage = {
			id: assistantMsgId,
			role: 'assistant',
			content: '',
			timestamp: new Date()
		};

		// Build conversation history for multi-turn testing (only completed prior messages + new user message)
		const apiMessages: ChatMessage[] = [
			...messages
				.filter((m) => m.content.trim().length > 0)
				.map((m) => ({
					role: m.role,
					content: m.content
				})),
			{ role: 'user', content: text }
		];

		messages = [...messages, userMsg, assistantMsg];
		isGenerating = true;

		// Scroll to bottom
		setTimeout(scrollToBottom, 20);

		abortController = new AbortController();
		const startTime = performance.now();

		// Use active key from keystore if available
		const activeKey = keysState.items.find((k) => k.isActive)?.key;
		const targetModel =
			selectedThinkingLevel !== 'auto' && thinkingLevels.includes(selectedThinkingLevel)
				? `${selectedModelId}(${selectedThinkingLevel})`
				: selectedModelId;

		try {
			await sendChatCompletion(
				{
					model: targetModel,
					messages: apiMessages,
					stream: true
				},
				activeKey,
				{
					signal: abortController.signal,
					onDelta: (delta) => {
						messages = messages.map((m) =>
							m.id === assistantMsgId ? { ...m, content: m.content + delta } : m
						);
						scrollToBottom();
					}
				}
			);

			const elapsed = Math.round(performance.now() - startTime);
			messages = messages.map((m) => (m.id === assistantMsgId ? { ...m, latencyMs: elapsed } : m));
		} catch (err: unknown) {
			if (err instanceof Error && err.name === 'AbortError') {
				// Aborted by user
				messages = messages.map((m) =>
					m.id === assistantMsgId && !m.content
						? { ...m, content: '[Generation stopped by user]' }
						: m
				);
			} else {
				const errMsg = err instanceof Error ? err.message : 'Chat completion failed';
				error = errMsg;
				messages = messages.filter((m) => m.id !== assistantMsgId || m.content.length > 0);
			}
		} finally {
			isGenerating = false;
			abortController = null;
			setTimeout(scrollToBottom, 50);
		}
	}

	function handleStop() {
		if (abortController) {
			abortController.abort();
		}
	}

	function handleClear() {
		if (isGenerating && abortController) {
			abortController.abort();
		}
		messages = [];
		error = null;
	}

	async function copyMessage(content: string, id: string) {
		try {
			await navigator.clipboard.writeText(content);
			copiedId = id;
			setTimeout(() => {
				copiedId = null;
			}, 2000);
		} catch (e: unknown) {
			console.error('Copy failed', e);
		}
	}

	const samplePrompts = [
		'Ping! Identify yourself and current model name.',
		'Explain quantum computing in 2 simple sentences.',
		'Write a short Python function to fetch JSON from a URL.'
	];
</script>

<Card.Root class="border-primary/20 bg-card shadow-xs transition-all">
	<!-- Playground Header -->
	<Card.Header class="border-b border-border/80 px-4 py-3 sm:px-5">
		<div class="flex flex-wrap items-center justify-between gap-3">
			<div class="flex items-center gap-2.5">
				<div
					class="flex size-7 items-center justify-center rounded-md border border-primary/25 bg-primary/10 text-primary shadow-xs"
				>
					<MessageSquareIcon class="size-3.5" />
				</div>
				<div class="flex flex-col">
					<div class="flex items-center gap-2">
						<Card.Title class="text-sm font-semibold tracking-tight">
							Temporary Model Chat
						</Card.Title>
						<Badge variant="outline" class="h-4 font-mono text-[9px] text-muted-foreground">
							In-Memory Test
						</Badge>
					</div>
					<Card.Description class="text-[11px] text-muted-foreground">
						Test responses and latency in real time. Conversation is ephemeral and not stored.
					</Card.Description>
				</div>
			</div>

			<!-- Controls: Model Select, Clear, Collapse -->
			<div class="flex items-center gap-2">
				<!-- Model selector dropdown -->
				<div class="flex items-center gap-1.5">
					<span class="hidden text-xs text-muted-foreground sm:inline">Model:</span>
					<Select.Root
						type="single"
						value={selectedModelId}
						onValueChange={(v) => {
							if (v) handleModelChange(v);
						}}
					>
						<Select.Trigger class="h-8 max-w-[200px] font-mono text-xs sm:max-w-xs">
							<Select.Value placeholder="Select model" />
						</Select.Trigger>
						<Select.Content class="max-h-72">
							<Select.Group>
								{#each modelList as m (m.id)}
									<Select.Item value={m.id} disabled={!m.enabled} label={m.id}>
										<span class="font-mono text-xs">{m.id}</span>
										{#if !m.enabled}
											<span class="text-[10px] text-muted-foreground">[Disabled]</span>
										{/if}
									</Select.Item>
								{/each}
							</Select.Group>
						</Select.Content>
					</Select.Root>
				</div>

				<!-- Thinking level selector dropdown -->
				{#if thinkingLevels.length > 0}
					<div class="flex items-center gap-1">
						<span class="hidden text-xs text-amber-600 sm:inline dark:text-amber-400"
							>Thinking:</span
						>
						<Select.Root type="single" bind:value={selectedThinkingLevel}>
							<Select.Trigger
								class="h-8 w-28 border-amber-500/30 bg-amber-500/10 font-mono text-xs text-amber-700 dark:text-amber-300"
								title="Thinking level / reasoning effort"
							>
								<Select.Value placeholder="auto" />
							</Select.Trigger>
							<Select.Content>
								<Select.Group>
									<Select.Item value="auto" label="auto">auto</Select.Item>
									{#each thinkingLevels as lvl (lvl)}
										<Select.Item value={lvl} label={lvl}>{lvl}</Select.Item>
									{/each}
								</Select.Group>
							</Select.Content>
						</Select.Root>
					</div>
				{/if}

				<!-- Clear button -->
				{#if messages.length > 0}
					<Button
						variant="ghost"
						size="sm"
						class="h-8 px-2 text-xs text-muted-foreground hover:text-destructive"
						onclick={handleClear}
						title="Clear chat history"
					>
						<Trash2Icon class="size-3.5" />
						<span class="hidden sm:inline">Clear</span>
					</Button>
				{/if}

				<!-- Collapse toggle -->
				<Button
					variant="ghost"
					size="icon-xs"
					class="h-8 w-8 text-muted-foreground"
					onclick={() => (isCollapsed = !isCollapsed)}
					title={isCollapsed ? 'Expand chat' : 'Collapse chat'}
				>
					{#if isCollapsed}
						<ChevronDownIcon class="size-4" />
					{:else}
						<ChevronUpIcon class="size-4" />
					{/if}
				</Button>
			</div>
		</div>
	</Card.Header>

	<!-- Chat Body & Input (hidden when collapsed) -->
	{#if !isCollapsed}
		<Card.Content class="flex flex-col gap-3 p-4 sm:p-5">
			{#if error}
				<Alert.Root variant="destructive" class="py-2 text-xs">
					<AlertCircleIcon class="size-4" />
					<Alert.Description class="text-xs">{error}</Alert.Description>
				</Alert.Root>
			{/if}

			<!-- Scrollable Messages Container -->
			<div
				bind:this={chatContainer}
				class="flex max-h-80 min-h-[160px] flex-col gap-3 overflow-y-auto rounded-lg border border-border/50 bg-muted/20 p-3 sm:p-4"
			>
				{#if messages.length === 0}
					<!-- Empty State with Quick Prompts -->
					<div
						class="my-auto flex flex-col items-center justify-center gap-3 py-6 text-center text-muted-foreground"
					>
						<div
							class="flex size-10 items-center justify-center rounded-full border border-border bg-card text-muted-foreground shadow-xs"
						>
							<SparklesIcon class="size-5 text-primary" />
						</div>
						<div class="flex max-w-sm flex-col gap-1">
							<span class="text-xs font-medium text-foreground">
								Ready to test {selectedModelId}
							</span>
							<p class="text-[11px] text-muted-foreground">
								Send a test prompt or pick one below to evaluate model streaming speed and output.
							</p>
						</div>

						<div class="flex flex-wrap justify-center gap-1.5 pt-1">
							{#each samplePrompts as promptText (promptText)}
								<button
									type="button"
									class="rounded-full border border-border bg-card px-2.5 py-1 text-start text-[11px] text-muted-foreground transition-colors hover:border-primary/50 hover:bg-primary/5 hover:text-foreground"
									onclick={() => handleSend(promptText)}
								>
									"{promptText}"
								</button>
							{/each}
						</div>
					</div>
				{:else}
					<!-- Message List -->
					{#each messages as msg (msg.id)}
						<div class={cn('flex gap-2.5', msg.role === 'user' ? 'justify-end' : 'justify-start')}>
							{#if msg.role === 'assistant'}
								<div
									class="flex size-6 shrink-0 items-center justify-center rounded-full border border-primary/20 bg-primary/10 text-primary shadow-xs"
									title={selectedModelId}
								>
									<BotIcon class="size-3.5" />
								</div>
							{/if}

							<div
								class={cn(
									'group relative flex flex-col gap-1 rounded-2xl px-3.5 py-2 text-xs transition-colors',
									msg.role === 'user'
										? 'max-w-[80%] rounded-tr-xs bg-primary text-primary-foreground'
										: 'max-w-[85%] rounded-tl-xs border border-border/60 bg-card text-card-foreground shadow-xs'
								)}
							>
								<!-- Message Content -->
								<div class="leading-relaxed break-words whitespace-pre-wrap">
									{#if msg.content}
										{msg.content}
									{:else if isGenerating && msg.role === 'assistant'}
										<span class="inline-flex items-center gap-1 text-muted-foreground">
											<RefreshCwIcon class="size-3 animate-spin" />
											Streaming response...
										</span>
									{/if}
								</div>

								<!-- Footer: Latency & Copy Button -->
								{#if msg.role === 'assistant' && msg.content}
									<div
										class="mt-1 flex items-center justify-between gap-3 border-t border-border/40 pt-1 text-[10px] text-muted-foreground"
									>
										{#if msg.latencyMs !== undefined}
											<span class="font-mono">{msg.latencyMs}ms</span>
										{:else}
											<span></span>
										{/if}

										<button
											type="button"
											class="flex items-center gap-1 rounded px-1 text-muted-foreground hover:text-foreground"
											onclick={() => copyMessage(msg.content, msg.id)}
											title="Copy message"
										>
											{#if copiedId === msg.id}
												<CheckIcon class="size-3 text-emerald-500" />
												<span class="text-emerald-500">Copied</span>
											{:else}
												<CopyIcon class="size-3 opacity-60 group-hover:opacity-100" />
												<span class="opacity-0 group-hover:opacity-100">Copy</span>
											{/if}
										</button>
									</div>
								{/if}
							</div>

							{#if msg.role === 'user'}
								<div
									class="flex size-6 shrink-0 items-center justify-center rounded-full border border-border bg-muted text-muted-foreground shadow-xs"
								>
									<UserIcon class="size-3.5" />
								</div>
							{/if}
						</div>
					{/each}
				{/if}
			</div>

			<!-- Input bar -->
			<form
				class="flex items-center gap-2"
				onsubmit={(e) => {
					e.preventDefault();
					handleSend();
				}}
			>
				<input
					type="text"
					bind:value={inputText}
					placeholder={`Message ${selectedModelId}...`}
					disabled={isGenerating}
					class="h-9 flex-1 rounded-md border border-input bg-background px-3 py-1 text-xs shadow-xs placeholder:text-muted-foreground focus-visible:ring-1 focus-visible:ring-ring focus-visible:outline-none disabled:opacity-50"
				/>

				{#if isGenerating}
					<Button
						type="button"
						variant="destructive"
						size="sm"
						class="h-9 shrink-0 gap-1.5 px-3 text-xs"
						onclick={handleStop}
					>
						<SquareIcon class="size-3 fill-current" />
						Stop
					</Button>
				{:else}
					<Button
						type="submit"
						variant="default"
						size="sm"
						disabled={!inputText.trim()}
						class="h-9 shrink-0 gap-1.5 px-3 text-xs"
					>
						<SendIcon class="size-3.5" />
						Send
					</Button>
				{/if}
			</form>
		</Card.Content>
	{/if}
</Card.Root>
