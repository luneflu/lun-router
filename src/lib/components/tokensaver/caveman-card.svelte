<script lang="ts">
	import { onMount } from 'svelte';
	import * as Card from '$lib/components/ui/card';
	import * as Select from '$lib/components/ui/select';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { Switch } from '$lib/components/ui/switch';
	import { cavemanState } from '$lib/state/caveman.svelte';
	import { keysState } from '$lib/state/keys.svelte';
	import { sendChatCompletion } from '$lib/api/completions';
	import { fetchProviderModels } from '$lib/api/providers';

	import CopyIcon from '@lucide/svelte/icons/copy';
	import CheckIcon from '@lucide/svelte/icons/check';
	import PlayIcon from '@lucide/svelte/icons/play';
	import SparklesIcon from '@lucide/svelte/icons/sparkles';
	import BotIcon from '@lucide/svelte/icons/bot';
	import RocketIcon from '@lucide/svelte/icons/rocket';
	import SquareIcon from '@lucide/svelte/icons/square';
	import AlertCircleIcon from '@lucide/svelte/icons/alert-circle';

	interface ModelOption {
		id: string;
		enabled: boolean;
	}

	let sampleType = $state<'react' | 'db' | 'go' | 'docker'>('react');
	let testPrompt = $state(getSample('react'));
	let copiedPrompt = $state(false);
	let copiedNormal = $state(false);
	let copiedCaveman = $state(false);
	let showFullPrompt = $state(false);

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

	// Live model comparison state: Without Caveman vs With Caveman
	let isNormalRunning = $state(false);
	let normalOutput = $state('');
	let normalLatencyMs = $state<number | null>(null);
	let normalError = $state<string | null>(null);

	let isCavemanRunning = $state(false);
	let cavemanOutput = $state('');
	let cavemanLatencyMs = $state<number | null>(null);
	let cavemanError = $state<string | null>(null);

	let abortController = $state<AbortController | null>(null);

	let isComparing = $derived(isNormalRunning || isCavemanRunning);
	let hasResults = $derived(!!(normalOutput || cavemanOutput || normalError || cavemanError));

	let normalWordCount = $derived(normalOutput.split(/\s+/).filter(Boolean).length);
	let cavemanWordCount = $derived(cavemanOutput.split(/\s+/).filter(Boolean).length);
	let wordsSaved = $derived(
		normalWordCount > cavemanWordCount ? normalWordCount - cavemanWordCount : 0
	);
	let savedPercent = $derived(
		normalWordCount > 0 && normalWordCount > cavemanWordCount
			? Math.round(((normalWordCount - cavemanWordCount) / normalWordCount) * 100)
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

	function getSample(type: 'react' | 'db' | 'go' | 'docker'): string {
		switch (type) {
			case 'react':
				return 'Why is my React component re-rendering on every user keystroke? How can I fix it?';
			case 'db':
				return 'Explain what database connection pooling is and why it is beneficial for high traffic apps.';
			case 'go':
				return 'How should I handle errors in Go idiomatic code? Why does Go avoid try-catch exceptions?';
			case 'docker':
				return 'Why is my Docker image 1.5GB and taking 3 minutes to build? How to optimize caching?';
		}
	}

	function handleSelectSample(type: 'react' | 'db' | 'go' | 'docker') {
		sampleType = type;
		testPrompt = getSample(type);
		cavemanState.testResult = null;
		resetComparison();
	}

	function resetComparison() {
		normalOutput = '';
		normalError = null;
		normalLatencyMs = null;
		cavemanOutput = '';
		cavemanError = null;
		cavemanLatencyMs = null;
	}

	async function handleRunSimulation() {
		resetComparison();
		await cavemanState.runTest(testPrompt, selectedProvider, selectedModel);
	}

	async function handleRunComparison() {
		if (isComparing) {
			abortController?.abort();
			isNormalRunning = false;
			isCavemanRunning = false;
			return;
		}

		resetComparison();
		cavemanState.testResult = null;
		isNormalRunning = true;
		isCavemanRunning = true;
		abortController = new AbortController();

		const activeKey = keysState.items.find((k) => k.isActive)?.key;

		// 1. Without Caveman (Normal Model Response)
		const runNormal = async () => {
			const start = performance.now();
			try {
				const full = await sendChatCompletion(
					{
						model: selectedModel,
						messages: [{ role: 'user', content: testPrompt }],
						stream: true
					},
					activeKey,
					{
						signal: abortController?.signal,
						headers: {
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

		// 2. With Caveman (Concise Smart Caveman Response)
		const runCaveman = async () => {
			const start = performance.now();
			try {
				const full = await sendChatCompletion(
					{
						model: selectedModel,
						messages: [{ role: 'user', content: testPrompt }],
						stream: true
					},
					activeKey,
					{
						signal: abortController?.signal,
						headers: {
							'X-Caveman': 'on',
							'X-Caveman-Mode': cavemanState.mode
						},
						onDelta: (delta: string) => {
							cavemanOutput += delta;
						}
					}
				);
				if (!cavemanOutput && full) {
					cavemanOutput = full;
				}
				cavemanLatencyMs = Math.round(performance.now() - start);
				if (!cavemanOutput) {
					cavemanError = 'Model returned empty response';
				}
			} catch (err: unknown) {
				if (err instanceof Error && err.name === 'AbortError') {
					if (!cavemanOutput) cavemanOutput = '[Stopped]';
				} else {
					cavemanError = err instanceof Error ? err.message : 'Execution failed';
				}
			} finally {
				isCavemanRunning = false;
			}
		};

		await Promise.allSettled([runNormal(), runCaveman()]);
		abortController = null;
	}

	async function handleCopyPrompt() {
		try {
			await navigator.clipboard.writeText(cavemanState.activePrompt);
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

	async function handleCopyCaveman() {
		try {
			await navigator.clipboard.writeText(cavemanOutput);
			copiedCaveman = true;
			setTimeout(() => (copiedCaveman = false), 2000);
		} catch {
			// ignore
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
						<Card.Title class="text-xl">Caveman Voice & Output Optimization</Card.Title>
						<Badge variant={cavemanState.enabled ? 'default' : 'outline'} class="text-[10px]">
							{cavemanState.enabled ? 'Active' : 'Disabled'}
						</Badge>
					</div>
					<Card.Description>
						Why many token when few do trick. Enforces ultra-concise system instructions to cut
						model response tokens by ~54% to 75% with zero loss of technical accuracy.
					</Card.Description>
				</div>

				<div class="flex items-center gap-3">
					<div class="flex items-center gap-2 rounded-lg border bg-muted/20 px-3 py-1.5">
						<Switch
							id="caveman-toggle"
							checked={cavemanState.enabled}
							onCheckedChange={() => cavemanState.toggleEnabled()}
							disabled={cavemanState.toggling}
						/>
						<label
							for="caveman-toggle"
							class="cursor-pointer text-xs font-medium text-muted-foreground select-none"
						>
							{cavemanState.enabled ? 'Enabled' : 'Disabled'}
						</label>
					</div>
				</div>
			</div>
		</Card.Header>

		{#if cavemanState.error}
			<div
				class="mx-6 mb-4 rounded-md border border-destructive/20 bg-destructive/10 px-3 py-2 text-xs text-destructive"
			>
				{cavemanState.error}
			</div>
		{/if}

		<Card.Content class="space-y-6">
			<!-- Metrics Row -->
			<div class="grid grid-cols-2 gap-4 md:grid-cols-4">
				<div class="rounded-lg border bg-card p-3">
					<div class="text-xs font-medium text-muted-foreground">Total Requests</div>
					<div class="mt-1 text-2xl font-bold tracking-tight">
						{cavemanState.stats.total_requests.toLocaleString()}
					</div>
					<div class="mt-0.5 text-[11px] text-muted-foreground">Through Lun Router</div>
				</div>

				<div class="rounded-lg border bg-card p-3">
					<div class="text-xs font-medium text-muted-foreground">Caveman Injections</div>
					<div class="mt-1 text-2xl font-bold tracking-tight text-primary">
						{cavemanState.stats.injected_requests.toLocaleString()}
					</div>
					<div class="mt-0.5 text-[11px] text-muted-foreground">Optimized prompts</div>
				</div>

				<div class="rounded-lg border bg-card p-3">
					<div class="text-xs font-medium text-muted-foreground">Est. Output Saved</div>
					<div class="mt-1 text-2xl font-bold tracking-tight text-emerald-500">
						~{cavemanState.stats.estimated_saved_pct}%
					</div>
					<div class="mt-0.5 text-[11px] text-muted-foreground">Fewer response tokens</div>
				</div>

				<div class="rounded-lg border bg-card p-3">
					<div class="text-xs font-medium text-muted-foreground">Prompt Source</div>
					<div class="mt-1 truncate text-sm font-semibold" title="Compiled into the router binary">
						Built-in
					</div>
					<div class="mt-0.5 truncate text-[11px] text-muted-foreground">
						{cavemanState.mode} · {cavemanState.activePrompt.length.toLocaleString()} chars
					</div>
				</div>
			</div>

			<!-- Intensity / Mode Selector -->
			<div class="space-y-3">
				<div class="flex items-center justify-between">
					<span class="text-xs font-semibold tracking-wider text-muted-foreground uppercase">
						Compression Intensity Mode
					</span>
					<span class="text-xs text-muted-foreground">
						Client override via <code class="rounded bg-muted px-1 py-0.5 font-mono text-[11px]"
							>X-Caveman-Mode</code
						> header
					</span>
				</div>

				<div class="grid grid-cols-1 gap-3 sm:grid-cols-3">
					{#each cavemanState.availableModes.filter((m) => m.id !== 'custom') as m (m.id)}
						<button
							type="button"
							onclick={() => cavemanState.setMode(m.id)}
							class={`flex flex-col items-start rounded-lg border p-3.5 text-left transition-all ${
								cavemanState.mode === m.id
									? 'border-primary bg-primary/5 shadow-sm'
									: 'border-border/60 hover:border-border hover:bg-muted/30'
							}`}
						>
							<div class="flex w-full items-center justify-between">
								<span class="text-sm font-semibold">{m.name}</span>
								{#if m.tokens_est > 0}
									<Badge variant="outline" class="font-mono text-[10px]">
										~{m.tokens_est} tokens
									</Badge>
								{/if}
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
						Active System Directive ({cavemanState.mode})
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
					<pre class="font-sans whitespace-pre-wrap">{cavemanState.activePrompt}</pre>
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
						Run live requests with and without Caveman side-by-side to measure exact token
						reductions.
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
					<Select.Root type="single" bind:value={selectedModel}>
						<Select.Trigger class="h-8 min-w-[200px] font-mono text-xs">
							<Select.Value placeholder="Select model" />
						</Select.Trigger>
						<Select.Content class="max-h-72">
							{#if enabledModels.length > 0}
								<Select.Group>
									<Select.GroupHeading>Enabled Models</Select.GroupHeading>
									{#each enabledModels as m (m.id)}
										<Select.Item value={m.id} label={m.id}>
											<span class="font-mono text-xs">{m.id}</span>
										</Select.Item>
									{/each}
								</Select.Group>
							{/if}
							{#if enabledModels.length > 0 && disabledModels.length > 0}
								<Select.Separator />
							{/if}
							{#if disabledModels.length > 0}
								<Select.Group>
									<Select.GroupHeading>Disabled Models</Select.GroupHeading>
									{#each disabledModels as m (m.id)}
										<Select.Item value={m.id} disabled label={m.id}>
											<span class="font-mono text-xs text-muted-foreground">{m.id} (disabled)</span>
										</Select.Item>
									{/each}
								</Select.Group>
							{/if}
						</Select.Content>
					</Select.Root>
				</div>
			</div>
		</Card.Header>

		<Card.Content class="space-y-4">
			<!-- Quick Sample Presets -->
			<div class="flex flex-wrap items-center justify-between gap-2">
				<span class="text-xs font-medium text-muted-foreground">Sample Prompts:</span>
				<div class="flex flex-wrap gap-1.5">
					<Button
						variant={sampleType === 'react' ? 'default' : 'outline'}
						size="sm"
						class="h-7 text-xs"
						onclick={() => handleSelectSample('react')}
					>
						React Render
					</Button>
					<Button
						variant={sampleType === 'db' ? 'default' : 'outline'}
						size="sm"
						class="h-7 text-xs"
						onclick={() => handleSelectSample('db')}
					>
						DB Pool
					</Button>
					<Button
						variant={sampleType === 'go' ? 'default' : 'outline'}
						size="sm"
						class="h-7 text-xs"
						onclick={() => handleSelectSample('go')}
					>
						Go Errors
					</Button>
					<Button
						variant={sampleType === 'docker' ? 'default' : 'outline'}
						size="sm"
						class="h-7 text-xs"
						onclick={() => handleSelectSample('docker')}
					>
						Docker Cache
					</Button>
				</div>
			</div>

			<!-- User Prompt Textarea -->
			<div class="space-y-2">
				<label for="prompt-input" class="text-xs font-medium text-muted-foreground">
					User Question / Prompt:
				</label>
				<textarea
					id="prompt-input"
					bind:value={testPrompt}
					rows={3}
					placeholder="Type any coding or technical question..."
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
						disabled={cavemanState.testing || isComparing || !testPrompt.trim()}
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
								Mode: {cavemanState.mode}
							</Badge>
						</div>

						{#if normalWordCount > 0 && cavemanWordCount > 0}
							<div>
								{#if savedPercent > 0}
									<Badge
										variant="default"
										class="gap-1 bg-emerald-600 font-mono text-[11px] text-white hover:bg-emerald-600"
									>
										✨ -{savedPercent}% output words saved (~{wordsSaved} words / ~{Math.round(
											wordsSaved * 1.33
										)} tokens)
									</Badge>
								{:else}
									<Badge variant="secondary" class="text-[10px]">Equal response length</Badge>
								{/if}
							</div>
						{/if}
					</div>

					<div class="grid grid-cols-1 gap-4 lg:grid-cols-2">
						<!-- Left Column: Without Caveman (Normal / Verbose) -->
						<div class="space-y-2 rounded-lg border bg-card p-3.5">
							<div class="flex items-center justify-between border-b pb-2 text-xs">
								<div class="flex items-center gap-1.5 font-semibold text-muted-foreground">
									<span class="inline-block h-2 w-2 rounded-full bg-zinc-500"></span>
									<span>Without Caveman (Standard)</span>
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
											(isNormalRunning ? 'Streaming normal verbose response...' : '')}</pre>
								</div>
							{/if}
						</div>

						<!-- Right Column: With Caveman (Terse Smart Caveman) -->
						<div class="space-y-2 rounded-lg border border-primary/30 bg-primary/[0.02] p-3.5">
							<div
								class="flex items-center justify-between border-b border-primary/20 pb-2 text-xs"
							>
								<div class="flex items-center gap-1.5 font-semibold text-primary">
									<span class="inline-block h-2 w-2 rounded-full bg-emerald-500"></span>
									<span>With Caveman ({cavemanState.mode})</span>
									{#if savedPercent > 0}
										<Badge
											variant="outline"
											class="h-4 border-emerald-500/30 px-1 py-0 text-[10px] text-emerald-500"
										>
											-{savedPercent}%
										</Badge>
									{/if}
								</div>
								<div class="flex items-center gap-2 font-mono text-[11px] text-muted-foreground">
									{#if cavemanLatencyMs !== null}
										<span class="font-semibold text-emerald-500">{cavemanLatencyMs}ms</span>
									{/if}
									{#if cavemanOutput}
										<span class="font-semibold text-emerald-500">{cavemanWordCount} words</span>
										<Button
											variant="ghost"
											size="sm"
											class="h-6 px-1 text-xs"
											onclick={handleCopyCaveman}
										>
											{#if copiedCaveman}
												<CheckIcon class="h-3 w-3 text-emerald-500" />
											{:else}
												<CopyIcon class="h-3 w-3" />
											{/if}
										</Button>
									{/if}
								</div>
							</div>

							{#if cavemanError}
								<div
									class="flex items-center gap-2 rounded border border-destructive/20 bg-destructive/10 p-2.5 text-xs text-destructive"
								>
									<AlertCircleIcon class="h-3.5 w-3.5 shrink-0" />
									<span>{cavemanError}</span>
								</div>
							{:else}
								<div
									class="max-h-72 overflow-y-auto rounded bg-zinc-950 p-3 font-mono text-xs leading-relaxed text-zinc-100"
								>
									<pre class="whitespace-pre-wrap">{cavemanOutput ||
											(isCavemanRunning
												? 'Streaming smart caveman response (no fluff)...'
												: '')}</pre>
								</div>
							{/if}
						</div>
					</div>
				</div>
			{/if}

			<!-- Simulation Preview Box -->
			{#if cavemanState.testResult}
				<div class="space-y-4 rounded-lg border bg-muted/20 p-4">
					<div class="flex flex-wrap items-center justify-between gap-2 border-b pb-3">
						<div class="flex items-center gap-2">
							<SparklesIcon class="h-4 w-4 text-primary" />
							<span class="text-xs font-semibold">Simulated Completion Payload</span>
							<Badge variant="outline" class="font-mono text-[10px]">
								Provider: {cavemanState.testResult.provider || selectedProvider}
							</Badge>
							<Badge variant="outline" class="font-mono text-[10px]">
								Model: {cavemanState.testResult.model || selectedModel}
							</Badge>
							<Badge variant="secondary" class="font-mono text-[10px]">
								Mode: {cavemanState.testResult.mode}
							</Badge>
						</div>
						<div class="text-xs font-medium text-emerald-500">
							{cavemanState.testResult.estimated_savings}
						</div>
					</div>

					<div class="grid grid-cols-1 gap-4 md:grid-cols-2">
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
								<pre class="whitespace-pre-wrap">{cavemanState.testResult.injected_prompt}</pre>
							</div>
						</div>

						<!-- Caveman Voice Output Comparison Benchmark -->
						<div class="space-y-1.5">
							<div
								class="flex items-center justify-between text-[11px] font-medium text-muted-foreground"
							>
								<span>Typical Output Comparison</span>
								<Badge variant="secondary" class="text-[10px]">Benchmark</Badge>
							</div>
							<div class="space-y-2 rounded-md border bg-card p-3 text-xs">
								<div class="rounded border border-red-500/20 bg-red-500/5 p-2">
									<div class="text-[10px] font-semibold text-red-400">
										Normal Agent (60-80 tokens):
									</div>
									<p class="mt-1 text-[11px] text-muted-foreground italic">
										"The reason your component is re-rendering is likely because an inline object or
										function creates a new reference on each render cycle. When passed down as
										props, shallow equality check fails..."
									</p>
								</div>
								<div class="rounded border border-emerald-500/20 bg-emerald-500/5 p-2">
									<div class="text-[10px] font-semibold text-emerald-400">
										Caveman Agent (~14-20 tokens):
									</div>
									<p class="mt-1 font-mono text-[11px] font-semibold text-foreground">
										{#if cavemanState.mode === 'ultracave'}
											"Inline object prop, new ref, re-render. Wrap in `useMemo`."
										{:else if cavemanState.mode === 'megacave'}
											"物出新參照，致重繪。以 `useMemo` 包之。"
										{:else}
											"New object ref each render, so React re-renders. Wrap prop in `useMemo`."
										{/if}
									</p>
								</div>
							</div>
						</div>
					</div>
				</div>
			{/if}
		</Card.Content>
	</Card.Root>
</div>
