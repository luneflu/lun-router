<script lang="ts">
	import { onMount } from 'svelte';
	import * as Card from '$lib/components/ui/card';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { Switch } from '$lib/components/ui/switch';
	import { ponytailState } from '$lib/state/ponytail.svelte';
	import { keysState } from '$lib/state/keys.svelte';
	import { sendChatCompletion } from '$lib/api/completions';
	import { fetchProviderModels } from '$lib/api/providers';

	import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';
	import ExternalLinkIcon from '@lucide/svelte/icons/external-link';
	import CopyIcon from '@lucide/svelte/icons/copy';
	import CheckIcon from '@lucide/svelte/icons/check';
	import PlayIcon from '@lucide/svelte/icons/play';
	import SparklesIcon from '@lucide/svelte/icons/sparkles';
	import GitCommitIcon from '@lucide/svelte/icons/git-commit';
	import BotIcon from '@lucide/svelte/icons/bot';
	import RocketIcon from '@lucide/svelte/icons/rocket';
	import SquareIcon from '@lucide/svelte/icons/square';
	import AlertCircleIcon from '@lucide/svelte/icons/alert-circle';
	import ScissorsIcon from '@lucide/svelte/icons/scissors';

	interface ModelOption {
		id: string;
		enabled: boolean;
	}

	let sampleType = $state<'ratelimit' | 'auth' | 'clone' | 'dialog'>('ratelimit');
	let testPrompt = $state(getSample('ratelimit'));
	let copiedPrompt = $state(false);
	let copiedNormal = $state(false);
	let copiedPonytail = $state(false);
	let showFullPrompt = $state(false);
	let syncSuccessMsg = $state<string | null>(null);

	// Provider & Model selection for live testing
	let selectedProvider = $state<'opencode' | 'antigravity'>('opencode');
	let selectedModel = $state('muse-spark-1.3-contributor-free');
	let openCodeModels = $state<ModelOption[]>([
		{ id: 'muse-spark-1.3-contributor-free', enabled: true },
		{ id: 'deepseek-v4.1-flash-free', enabled: true },
		{ id: 'space-bunny-free', enabled: true },
		{ id: 'big-pickle', enabled: false }
	]);
	let antigravityModels = $state<ModelOption[]>([
		{ id: 'gemini-3.8-flash-high', enabled: true },
		{ id: 'claude-sonnet-4-6', enabled: true },
		{ id: 'gemini-3.8-flash', enabled: false }
	]);

	// Live model comparison state: Without Ponytail vs With Ponytail
	let isNormalRunning = $state(false);
	let normalOutput = $state('');
	let normalLatencyMs = $state<number | null>(null);
	let normalError = $state<string | null>(null);

	let isPonytailRunning = $state(false);
	let ponytailOutput = $state('');
	let ponytailLatencyMs = $state<number | null>(null);
	let ponytailError = $state<string | null>(null);

	let abortController = $state<AbortController | null>(null);

	let isComparing = $derived(isNormalRunning || isPonytailRunning);
	let hasResults = $derived(!!(normalOutput || ponytailOutput || normalError || ponytailError));

	let normalWordCount = $derived(normalOutput.split(/\s+/).filter(Boolean).length);
	let ponytailWordCount = $derived(ponytailOutput.split(/\s+/).filter(Boolean).length);
	let wordsDiff = $derived(
		normalWordCount > ponytailWordCount ? normalWordCount - ponytailWordCount : 0
	);
	let reductionPct = $derived(
		normalWordCount > 0 && normalWordCount > ponytailWordCount
			? Math.round(((normalWordCount - ponytailWordCount) / normalWordCount) * 100)
			: 0
	);

	let currentModels = $derived(
		selectedProvider === 'opencode' ? openCodeModels : antigravityModels
	);
	let enabledModels = $derived(currentModels.filter((m) => m.enabled));
	let disabledModels = $derived(currentModels.filter((m) => !m.enabled));

	onMount(() => {
		keysState.load();
		loadAllModels();
	});

	async function loadAllModels() {
		try {
			const [ocRes, agRes] = await Promise.allSettled([
				fetchProviderModels('opencode'),
				fetchProviderModels('antigravity')
			]);

			if (ocRes.status === 'fulfilled' && ocRes.value.models.length > 0) {
				openCodeModels = ocRes.value.models.map((m) => ({ id: m.id, enabled: m.enabled }));
			}
			if (agRes.status === 'fulfilled' && agRes.value.models.length > 0) {
				antigravityModels = agRes.value.models.map((m) => ({ id: m.id, enabled: m.enabled }));
			}

			pickFirstEnabled();
		} catch {
			// fallback
		}
	}

	function pickFirstEnabled() {
		const list = selectedProvider === 'opencode' ? openCodeModels : antigravityModels;
		const firstEnabled = list.find((m) => m.enabled);
		if (firstEnabled) {
			selectedModel = firstEnabled.id;
		} else if (list.length > 0) {
			selectedModel = list[0].id;
		}
	}

	function handleProviderChange(prov: 'opencode' | 'antigravity') {
		selectedProvider = prov;
		pickFirstEnabled();
	}

	function getSample(type: 'ratelimit' | 'auth' | 'clone' | 'dialog'): string {
		switch (type) {
			case 'ratelimit':
				return 'Write a simple HTTP rate limiter in Go for 10 requests per second per IP as a small, self-contained runnable example.';
			case 'auth':
				return 'Build a lightweight JWT authentication middleware in TypeScript as a small, self-contained runnable example.';
			case 'clone':
				return 'How do I deep clone an object in modern JavaScript without libraries? Provide a small, self-contained example.';
			case 'dialog':
				return 'How do I implement a lightweight modal dialog in HTML/CSS/JS? Provide a small, self-contained example.';
		}
	}

	function handleSelectSample(type: 'ratelimit' | 'auth' | 'clone' | 'dialog') {
		sampleType = type;
		testPrompt = getSample(type);
		ponytailState.testResult = null;
		resetComparison();
	}

	function resetComparison() {
		normalOutput = '';
		normalError = null;
		normalLatencyMs = null;
		ponytailOutput = '';
		ponytailError = null;
		ponytailLatencyMs = null;
	}

	function getEffectivePrompt(input: string): string {
		const lower = input.toLowerCase();
		if (
			lower.includes('self-contained') ||
			lower.includes('small example') ||
			lower.includes('standalone')
		) {
			return input;
		}
		return `${input}\n\n(Provide a small, self-contained runnable example directly; no repository or directory scanning required)`;
	}

	async function handleRunSimulation() {
		resetComparison();
		const promptToSend = getEffectivePrompt(testPrompt);
		await ponytailState.runTest(promptToSend, selectedProvider, selectedModel);
	}

	async function handleRunComparison() {
		if (isComparing) {
			abortController?.abort();
			isNormalRunning = false;
			isPonytailRunning = false;
			return;
		}

		resetComparison();
		ponytailState.testResult = null;
		isNormalRunning = true;
		isPonytailRunning = true;
		abortController = new AbortController();

		const activeKey = keysState.items.find((k) => k.isActive)?.key;
		const promptToSend = getEffectivePrompt(testPrompt);

		// 1. Without Ponytail (Standard response with potential over-engineering)
		const runNormal = async () => {
			const start = performance.now();
			try {
				const full = await sendChatCompletion(
					{
						model: selectedModel,
						messages: [{ role: 'user', content: promptToSend }],
						stream: true
					},
					activeKey,
					{
						signal: abortController?.signal,
						headers: {
							'X-Ponytail': 'off',
							'X-Caveman': 'off'
						},
						onDelta: (delta: string) => {
							normalOutput += delta;
						}
					}
				);
				if (!normalOutput && full) {
					normalOutput = full;
				}
				normalLatencyMs = Math.round(performance.now() - start);
				if (!normalOutput) {
					normalError = 'Model returned empty response';
				}
			} catch (err: unknown) {
				if (err instanceof Error && err.name === 'AbortError') {
					if (!normalOutput) normalOutput = '[Stopped]';
				} else {
					normalError = err instanceof Error ? err.message : 'Execution failed';
				}
			} finally {
				isNormalRunning = false;
			}
		};

		// 2. With Ponytail (Lazy Senior Dev: smallest change, anti-bloat)
		const runPonytail = async () => {
			const start = performance.now();
			try {
				const full = await sendChatCompletion(
					{
						model: selectedModel,
						messages: [{ role: 'user', content: promptToSend }],
						stream: true
					},
					activeKey,
					{
						signal: abortController?.signal,
						headers: {
							'X-Ponytail': 'on',
							'X-Ponytail-Mode': ponytailState.mode,
							'X-Caveman': 'off'
						},
						onDelta: (delta: string) => {
							ponytailOutput += delta;
						}
					}
				);
				if (!ponytailOutput && full) {
					ponytailOutput = full;
				}
				ponytailLatencyMs = Math.round(performance.now() - start);
				if (!ponytailOutput) {
					ponytailError = 'Model returned empty response';
				}
			} catch (err: unknown) {
				if (err instanceof Error && err.name === 'AbortError') {
					if (!ponytailOutput) ponytailOutput = '[Stopped]';
				} else {
					ponytailError = err instanceof Error ? err.message : 'Execution failed';
				}
			} finally {
				isPonytailRunning = false;
			}
		};

		await Promise.allSettled([runNormal(), runPonytail()]);
		abortController = null;
	}

	async function handleSyncUpstream() {
		syncSuccessMsg = null;
		try {
			const updated = await ponytailState.syncUpstream();
			if (updated) {
				syncSuccessMsg = 'Successfully pulled latest rules from upstream GitHub!';
			} else {
				syncSuccessMsg = 'Already up to date with latest upstream commit.';
			}
			setTimeout(() => {
				syncSuccessMsg = null;
			}, 4000);
		} catch {
			// error stored in ponytailState.error
		}
	}

	async function handleCopyPrompt() {
		try {
			await navigator.clipboard.writeText(ponytailState.activePrompt);
			copiedPrompt = true;
			setTimeout(() => (copiedPrompt = false), 2000);
		} catch {
			// ignore
		}
	}

	async function handleCopyNormal() {
		try {
			await navigator.clipboard.writeText(normalOutput);
			copiedNormal = true;
			setTimeout(() => (copiedNormal = false), 2000);
		} catch {
			// ignore
		}
	}

	async function handleCopyPonytail() {
		try {
			await navigator.clipboard.writeText(ponytailOutput);
			copiedPonytail = true;
			setTimeout(() => (copiedPonytail = false), 2000);
		} catch {
			// ignore
		}
	}

	function formatDateTime(iso: string | null): string {
		if (!iso) return 'Never';
		try {
			const d = new Date(iso);
			return d.toLocaleString(undefined, {
				month: 'short',
				day: 'numeric',
				hour: '2-digit',
				minute: '2-digit'
			});
		} catch {
			return iso;
		}
	}
</script>

<div class="space-y-6">
	<!-- Top Controls Card -->
	<Card.Root>
		<Card.Header>
			<div class="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
				<div class="space-y-1">
					<div class="flex flex-wrap items-center gap-2">
						<Card.Title class="text-xl">Ponytail Lazy Senior Developer Guidance</Card.Title>
						<Badge variant={ponytailState.enabled ? 'default' : 'outline'} class="text-[10px]">
							{ponytailState.enabled ? 'Active' : 'Disabled'}
						</Badge>
						{#if ponytailState.upstreamCommit}
							<a
								href={`${ponytailState.upstreamURL}/commit/${ponytailState.upstreamCommit}`}
								target="_blank"
								rel="noreferrer"
								class="inline-flex items-center gap-1 rounded border border-border/60 bg-muted/40 px-2 py-0.5 font-mono text-[10px] text-muted-foreground transition-colors hover:text-foreground"
							>
								<GitCommitIcon class="h-3 w-3" />
								{ponytailState.upstreamCommit}
								<ExternalLinkIcon class="h-2.5 w-2.5" />
							</a>
						{/if}
						<Badge variant="secondary" class="text-[10px]">Auto-Synced</Badge>
					</div>
					<Card.Description>
						The best code is the code you never wrote. Makes your AI agent think like the laziest
						senior dev: smallest complete change, YAGNI, standard library first, zero unasked
						abstractions.
					</Card.Description>
				</div>

				<div class="flex items-center gap-3">
					<Button
						variant="outline"
						size="sm"
						onclick={handleSyncUpstream}
						disabled={ponytailState.syncing}
						class="h-9 gap-1.5 text-xs"
					>
						<RefreshCwIcon class={`h-3.5 w-3.5 ${ponytailState.syncing ? 'animate-spin' : ''}`} />
						{ponytailState.syncing ? 'Syncing...' : 'Sync with GitHub'}
					</Button>

					<div class="flex items-center gap-2 rounded-lg border bg-muted/20 px-3 py-1.5">
						<Switch
							id="ponytail-toggle"
							checked={ponytailState.enabled}
							onCheckedChange={() => ponytailState.toggleEnabled()}
							disabled={ponytailState.toggling}
						/>
						<label
							for="ponytail-toggle"
							class="cursor-pointer text-xs font-medium text-muted-foreground select-none"
						>
							{ponytailState.enabled ? 'Enabled' : 'Disabled'}
						</label>
					</div>
				</div>
			</div>
		</Card.Header>

		{#if syncSuccessMsg}
			<div
				class="mx-6 mb-4 rounded-md border border-emerald-500/20 bg-emerald-500/10 px-3 py-2 text-xs text-emerald-400"
			>
				{syncSuccessMsg}
			</div>
		{/if}

		{#if ponytailState.error}
			<div
				class="mx-6 mb-4 rounded-md border border-destructive/20 bg-destructive/10 px-3 py-2 text-xs text-destructive"
			>
				{ponytailState.error}
			</div>
		{/if}

		<Card.Content class="space-y-6">
			<!-- Metrics Row -->
			<div class="grid grid-cols-2 gap-4 md:grid-cols-4">
				<div class="rounded-lg border bg-card p-3">
					<div class="text-xs font-medium text-muted-foreground">Total Requests</div>
					<div class="mt-1 text-2xl font-bold tracking-tight">
						{ponytailState.stats.total_requests.toLocaleString()}
					</div>
					<div class="mt-0.5 text-[11px] text-muted-foreground">Through Lun Router</div>
				</div>

				<div class="rounded-lg border bg-card p-3">
					<div class="text-xs font-medium text-muted-foreground">Ponytail Injections</div>
					<div class="mt-1 text-2xl font-bold tracking-tight text-primary">
						{ponytailState.stats.injected_requests.toLocaleString()}
					</div>
					<div class="mt-0.5 text-[11px] text-muted-foreground">Anti-bloat directives</div>
				</div>

				<div class="rounded-lg border bg-card p-3">
					<div class="text-xs font-medium text-muted-foreground">Guiding Philosophy</div>
					<div class="mt-1 text-sm font-semibold tracking-tight text-emerald-500">
						Smallest Working Diff
					</div>
					<div class="mt-0.5 text-[11px] text-muted-foreground">YAGNI & zero boilerplate</div>
				</div>

				<div class="rounded-lg border bg-card p-3">
					<div class="text-xs font-medium text-muted-foreground">Upstream Sync</div>
					<div
						class="mt-1 truncate text-sm font-semibold"
						title={ponytailState.upstreamCommitMsg || 'Up to date'}
					>
						{ponytailState.upstreamCommit ? `Commit ${ponytailState.upstreamCommit}` : 'Bundled'}
					</div>
					<div class="mt-0.5 truncate text-[11px] text-muted-foreground">
						{formatDateTime(ponytailState.lastSync)}
					</div>
				</div>
			</div>

			<!-- Intensity / Mode Selector -->
			<div class="space-y-3">
				<div class="flex items-center justify-between">
					<span class="text-xs font-semibold tracking-wider text-muted-foreground uppercase">
						Ponytail Guidance Mode
					</span>
					<span class="text-xs text-muted-foreground">
						Client override via <code class="rounded bg-muted px-1 py-0.5 font-mono text-[11px]"
							>X-Ponytail-Mode</code
						> header
					</span>
				</div>

				<div class="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-4">
					{#each ponytailState.availableModes.filter((m) => m.id !== 'custom') as m (m.id)}
						<button
							type="button"
							onclick={() => ponytailState.setMode(m.id)}
							class={`flex flex-col items-start rounded-lg border p-3.5 text-left transition-all ${
								ponytailState.mode === m.id
									? 'border-primary bg-primary/5 shadow-sm'
									: 'border-border/60 hover:border-border hover:bg-muted/30'
							}`}
						>
							<div class="flex w-full items-center justify-between">
								<span class="text-sm font-semibold">{m.name}</span>
								<ScissorsIcon class="h-3.5 w-3.5 text-muted-foreground" />
							</div>
							<p class="mt-1.5 text-xs leading-relaxed text-muted-foreground">
								{m.description}
							</p>
						</button>
					{/each}
				</div>
			</div>

			<!-- Active Directive Box -->
			<div class="space-y-2">
				<div class="flex items-center justify-between">
					<span class="text-xs font-semibold tracking-wider text-muted-foreground uppercase">
						Active Directive ({ponytailState.mode})
					</span>
					<div class="flex items-center gap-2">
						<Button
							variant="ghost"
							size="sm"
							class="h-6 text-xs text-muted-foreground hover:text-foreground"
							onclick={() => (showFullPrompt = !showFullPrompt)}
						>
							{showFullPrompt ? 'Collapse' : 'Expand full prompt'}
						</Button>
						<Button
							variant="ghost"
							size="sm"
							class="h-6 text-xs text-muted-foreground hover:text-foreground"
							onclick={handleCopyPrompt}
						>
							{#if copiedPrompt}
								<CheckIcon class="mr-1 h-3 w-3 text-emerald-500" />
								<span>Copied!</span>
							{:else}
								<CopyIcon class="mr-1 h-3 w-3" />
								<span>Copy</span>
							{/if}
						</Button>
					</div>
				</div>

				<div
					class={`rounded-lg border border-border/80 bg-zinc-950 p-3.5 font-mono text-[11px] leading-relaxed text-zinc-300 transition-all ${
						showFullPrompt ? 'max-h-[380px] overflow-y-auto' : 'max-h-[140px] overflow-hidden'
					}`}
				>
					<pre class="font-sans whitespace-pre-wrap">{ponytailState.activePrompt}</pre>
				</div>
			</div>
		</Card.Content>
	</Card.Root>

	<!-- Interactive Sandbox / Tester Card with Side-by-Side Comparison -->
	<Card.Root>
		<Card.Header>
			<div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
				<div>
					<Card.Title class="text-base font-semibold">Live Model Comparison</Card.Title>
					<Card.Description>
						Run live requests with and without Ponytail to compare code bloat and over-engineering.
					</Card.Description>
				</div>

				<!-- Provider Selection Buttons -->
				<div class="flex items-center gap-2">
					<div class="flex items-center rounded-lg border bg-muted/30 p-0.5">
						<button
							type="button"
							onclick={() => handleProviderChange('opencode')}
							class={`flex items-center gap-1.5 rounded-md px-2.5 py-1 text-xs font-medium transition-colors ${
								selectedProvider === 'opencode'
									? 'bg-background text-foreground shadow-sm'
									: 'text-muted-foreground hover:text-foreground'
							}`}
						>
							<BotIcon class="h-3.5 w-3.5 text-primary" />
							<span>OpenCode</span>
						</button>
						<button
							type="button"
							onclick={() => handleProviderChange('antigravity')}
							class={`flex items-center gap-1.5 rounded-md px-2.5 py-1 text-xs font-medium transition-colors ${
								selectedProvider === 'antigravity'
									? 'bg-background text-foreground shadow-sm'
									: 'text-muted-foreground hover:text-foreground'
							}`}
						>
							<RocketIcon class="h-3.5 w-3.5 text-amber-500" />
							<span>Antigravity</span>
						</button>
					</div>

					<!-- Model Select Menu -->
					<select
						bind:value={selectedModel}
						class="h-8 rounded-md border border-border bg-background px-2.5 font-mono text-xs focus:ring-1 focus:ring-ring focus:outline-none"
					>
						{#if enabledModels.length > 0}
							<optgroup label="Enabled Models">
								{#each enabledModels as m (m.id)}
									<option value={m.id}>{m.id}</option>
								{/each}
							</optgroup>
						{/if}
						{#if disabledModels.length > 0}
							<optgroup label="Disabled Models">
								{#each disabledModels as m (m.id)}
									<option value={m.id} disabled>{m.id} (disabled)</option>
								{/each}
							</optgroup>
						{/if}
					</select>
				</div>
			</div>
		</Card.Header>

		<Card.Content class="space-y-4">
			<!-- Quick Sample Presets -->
			<div class="flex flex-wrap items-center justify-between gap-2">
				<span class="text-xs font-medium text-muted-foreground">Sample Prompts:</span>
				<div class="flex flex-wrap gap-1.5">
					<Button
						variant={sampleType === 'ratelimit' ? 'default' : 'outline'}
						size="sm"
						class="h-7 text-xs"
						onclick={() => handleSelectSample('ratelimit')}
					>
						Rate Limiter
					</Button>
					<Button
						variant={sampleType === 'auth' ? 'default' : 'outline'}
						size="sm"
						class="h-7 text-xs"
						onclick={() => handleSelectSample('auth')}
					>
						Auth Layer
					</Button>
					<Button
						variant={sampleType === 'clone' ? 'default' : 'outline'}
						size="sm"
						class="h-7 text-xs"
						onclick={() => handleSelectSample('clone')}
					>
						Deep Clone
					</Button>
					<Button
						variant={sampleType === 'dialog' ? 'default' : 'outline'}
						size="sm"
						class="h-7 text-xs"
						onclick={() => handleSelectSample('dialog')}
					>
						Modal Dialog
					</Button>
				</div>
			</div>

			<!-- User Prompt Textarea -->
			<div class="space-y-2">
				<label for="ponytail-prompt-input" class="text-xs font-medium text-muted-foreground">
					User Request / Coding Task:
				</label>
				<textarea
					id="ponytail-prompt-input"
					bind:value={testPrompt}
					rows={3}
					placeholder="Type any software engineering or architectural request..."
					class="w-full rounded-md border border-border bg-background px-3 py-2 font-mono text-xs placeholder:text-muted-foreground/50 focus:ring-1 focus:ring-ring focus:outline-none"
				></textarea>
			</div>

			<!-- Action Buttons -->
			<div class="flex flex-wrap items-center justify-between gap-3">
				<div class="flex items-center gap-2 text-xs text-muted-foreground">
					<span>Selected:</span>
					<Badge variant="outline" class="font-mono text-[10px]">
						{selectedProvider} / {selectedModel}
					</Badge>
				</div>

				<div class="flex items-center gap-2">
					<Button
						variant="outline"
						size="sm"
						onclick={handleRunSimulation}
						disabled={ponytailState.testing || isComparing || !testPrompt.trim()}
						class="h-8 gap-1.5 text-xs"
					>
						<SparklesIcon class="h-3.5 w-3.5 text-primary" />
						<span>Simulate Directive</span>
					</Button>

					<Button
						size="sm"
						onclick={handleRunComparison}
						disabled={!testPrompt.trim()}
						class="h-8 gap-1.5 text-xs"
						variant={isComparing ? 'destructive' : 'default'}
					>
						{#if isComparing}
							<SquareIcon class="h-3.5 w-3.5 fill-current" />
							<span>Stop Stream</span>
						{:else}
							<PlayIcon class="h-3.5 w-3.5" />
							<span>Run Comparison (With vs Without)</span>
						{/if}
					</Button>
				</div>
			</div>

			<!-- Live Side-by-Side Comparison Output -->
			{#if isComparing || hasResults}
				<div class="space-y-4 rounded-lg border bg-muted/20 p-4">
					<div class="flex flex-wrap items-center justify-between gap-2 border-b pb-3">
						<div class="flex items-center gap-2">
							<SparklesIcon class="h-4 w-4 text-primary" />
							<span class="text-xs font-semibold">Live Side-by-Side Comparison</span>
							<Badge variant="secondary" class="font-mono text-[10px]">
								{selectedModel}
							</Badge>
							<Badge variant="outline" class="font-mono text-[10px]">
								Mode: {ponytailState.mode}
							</Badge>
						</div>

						{#if normalWordCount > 0 && ponytailWordCount > 0}
							<div>
								{#if reductionPct > 0}
									<Badge
										variant="default"
										class="gap-1 bg-emerald-600 font-mono text-[11px] text-white hover:bg-emerald-600"
									>
										✂️ -{reductionPct}% code bloat reduced (~{wordsDiff} words trimmed)
									</Badge>
								{:else}
									<Badge variant="secondary" class="text-[10px]">Equal response length</Badge>
								{/if}
							</div>
						{/if}
					</div>

					<div class="grid grid-cols-1 gap-4 lg:grid-cols-2">
						<!-- Left Column: Without Ponytail (Prone to over-engineering) -->
						<div class="space-y-2 rounded-lg border bg-card p-3.5">
							<div class="flex items-center justify-between border-b pb-2 text-xs">
								<div class="flex items-center gap-1.5 font-semibold text-muted-foreground">
									<span class="inline-block h-2 w-2 rounded-full bg-zinc-500"></span>
									<span>Without Ponytail (Standard / Bloated)</span>
								</div>
								<div class="flex items-center gap-2 font-mono text-[11px] text-muted-foreground">
									{#if normalLatencyMs !== null}
										<span>{normalLatencyMs}ms</span>
									{/if}
									{#if normalOutput}
										<span>{normalWordCount} words</span>
										<Button
											variant="ghost"
											size="sm"
											class="h-6 px-1 text-xs"
											onclick={handleCopyNormal}
										>
											{#if copiedNormal}
												<CheckIcon class="h-3 w-3 text-emerald-500" />
											{:else}
												<CopyIcon class="h-3 w-3" />
											{/if}
										</Button>
									{/if}
								</div>
							</div>

							{#if normalError}
								<div
									class="flex items-center gap-2 rounded border border-destructive/20 bg-destructive/10 p-2.5 text-xs text-destructive"
								>
									<AlertCircleIcon class="h-3.5 w-3.5 shrink-0" />
									<span>{normalError}</span>
								</div>
							{:else}
								<div
									class="max-h-72 overflow-y-auto rounded bg-zinc-950 p-3 font-mono text-xs leading-relaxed text-zinc-300"
								>
									<pre class="whitespace-pre-wrap">{normalOutput ||
											(isNormalRunning
												? 'Streaming standard response (interfaces, wrappers, boilerplate)...'
												: '')}</pre>
								</div>
							{/if}
						</div>

						<!-- Right Column: With Ponytail (Lean, Lazy Senior Dev) -->
						<div class="space-y-2 rounded-lg border border-primary/30 bg-primary/[0.02] p-3.5">
							<div
								class="flex items-center justify-between border-b border-primary/20 pb-2 text-xs"
							>
								<div class="flex items-center gap-1.5 font-semibold text-primary">
									<span class="inline-block h-2 w-2 rounded-full bg-emerald-500"></span>
									<span>With Ponytail ({ponytailState.mode})</span>
									{#if reductionPct > 0}
										<Badge
											variant="outline"
											class="h-4 border-emerald-500/30 px-1 py-0 text-[10px] text-emerald-500"
										>
											-{reductionPct}%
										</Badge>
									{/if}
								</div>
								<div class="flex items-center gap-2 font-mono text-[11px] text-muted-foreground">
									{#if ponytailLatencyMs !== null}
										<span class="font-semibold text-emerald-500">{ponytailLatencyMs}ms</span>
									{/if}
									{#if ponytailOutput}
										<span class="font-semibold text-emerald-500">{ponytailWordCount} words</span>
										<Button
											variant="ghost"
											size="sm"
											class="h-6 px-1 text-xs"
											onclick={handleCopyPonytail}
										>
											{#if copiedPonytail}
												<CheckIcon class="h-3 w-3 text-emerald-500" />
											{:else}
												<CopyIcon class="h-3 w-3" />
											{/if}
										</Button>
									{/if}
								</div>
							</div>

							{#if ponytailError}
								<div
									class="flex items-center gap-2 rounded border border-destructive/20 bg-destructive/10 p-2.5 text-xs text-destructive"
								>
									<AlertCircleIcon class="h-3.5 w-3.5 shrink-0" />
									<span>{ponytailError}</span>
								</div>
							{:else}
								<div
									class="max-h-72 overflow-y-auto rounded bg-zinc-950 p-3 font-mono text-xs leading-relaxed text-zinc-100"
								>
									<pre class="whitespace-pre-wrap">{ponytailOutput ||
											(isPonytailRunning
												? 'Streaming lazy senior developer response (YAGNI, minimal diff, no boilerplate)...'
												: '')}</pre>
								</div>
							{/if}
						</div>
					</div>
				</div>
			{/if}

			<!-- Simulation Preview Box -->
			{#if ponytailState.testResult}
				<div class="space-y-4 rounded-lg border bg-muted/20 p-4">
					<div class="flex flex-wrap items-center justify-between gap-2 border-b pb-3">
						<div class="flex items-center gap-2">
							<SparklesIcon class="h-4 w-4 text-primary" />
							<span class="text-xs font-semibold">Simulated Completion Payload</span>
							<Badge variant="outline" class="font-mono text-[10px]">
								Provider: {ponytailState.testResult.provider || selectedProvider}
							</Badge>
							<Badge variant="outline" class="font-mono text-[10px]">
								Model: {ponytailState.testResult.model || selectedModel}
							</Badge>
							<Badge variant="secondary" class="font-mono text-[10px]">
								Mode: {ponytailState.testResult.mode}
							</Badge>
						</div>
					</div>

					<!-- Injected System Directive Preview -->
					<div class="space-y-1.5">
						<div
							class="flex items-center justify-between text-[11px] font-medium text-muted-foreground"
						>
							<span>Injected System Directive</span>
							<span class="font-mono text-[10px]">role: system</span>
						</div>
						<div
							class="max-h-48 overflow-y-auto rounded-md border bg-zinc-950 p-2.5 font-mono text-[11px] text-zinc-300"
						>
							<pre class="whitespace-pre-wrap">{ponytailState.testResult.injected_prompt}</pre>
						</div>
					</div>
				</div>
			{/if}
		</Card.Content>
	</Card.Root>
</div>
