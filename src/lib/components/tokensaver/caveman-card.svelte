<script lang="ts">
	import { onMount } from 'svelte';
	import * as Card from '$lib/components/ui/card';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { Switch } from '$lib/components/ui/switch';
	import { cavemanState } from '$lib/state/caveman.svelte';
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

	let sampleType = $state<'react' | 'db' | 'go' | 'docker'>('react');
	let testPrompt = $state(getSample('react'));
	let copiedPrompt = $state(false);
	let copiedOutput = $state(false);
	let showFullPrompt = $state(false);
	let syncSuccessMsg = $state<string | null>(null);

	// Provider & Model selection for live testing
	let selectedProvider = $state<'opencode' | 'antigravity'>('opencode');
	let selectedModel = $state('big-pickle');
	let openCodeModels = $state<string[]>([
		'big-pickle',
		'muse-spark-1.3-contributor-free',
		'nemotron-3.5-lightning-free',
		'cat-jam-free',
		'mimo-v2.6-flash-free',
		'space-bunny-free',
		'deepseek-chat',
		'gpt-4o',
		'claude-3-5-sonnet'
	]);
	const antigravityModels = [
		'gemini-3.8-flash',
		'gemini-3.8-pro',
		'gemini-3.8-ultra',
		'claude-3-7-sonnet',
		'claude-3-5-sonnet',
		'gemini-2.5-flash',
		'gemini-2.5-pro'
	];

	// Live model execution state
	let isLiveRunning = $state(false);
	let liveOutput = $state('');
	let liveLatencyMs = $state<number | null>(null);
	let liveError = $state<string | null>(null);
	let abortController = $state<AbortController | null>(null);

	let currentModels = $derived(
		selectedProvider === 'opencode' ? openCodeModels : antigravityModels
	);

	onMount(() => {
		fetchProviderModels('opencode')
			.then((res) => {
				if (res.models && res.models.length > 0) {
					const ids = res.models.filter((m) => m.enabled).map((m) => m.id);
					if (ids.length > 0) {
						openCodeModels = Array.from(new Set([...ids, ...openCodeModels]));
					}
				}
			})
			.catch(() => {
				// use defaults
			});
	});

	function handleProviderChange(prov: 'opencode' | 'antigravity') {
		selectedProvider = prov;
		if (prov === 'antigravity') {
			selectedModel = 'gemini-3.8-flash';
		} else {
			selectedModel = 'big-pickle';
		}
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
		liveOutput = '';
		liveError = null;
		liveLatencyMs = null;
	}

	async function handleRunSimulation() {
		liveOutput = '';
		liveError = null;
		liveLatencyMs = null;
		await cavemanState.runTest(testPrompt, selectedProvider, selectedModel);
	}

	async function handleLiveRun() {
		if (isLiveRunning) {
			abortController?.abort();
			isLiveRunning = false;
			return;
		}

		isLiveRunning = true;
		liveOutput = '';
		liveError = null;
		liveLatencyMs = null;
		abortController = new AbortController();

		const start = performance.now();

		try {
			await sendChatCompletion(
				{
					model: selectedModel,
					messages: [{ role: 'user', content: testPrompt }],
					stream: true
				},
				undefined,
				{
					signal: abortController.signal,
					headers: {
						'X-Caveman': 'on',
						'X-Caveman-Mode': cavemanState.mode
					},
					onDelta: (delta: string) => {
						liveOutput += delta;
					}
				}
			);
			liveLatencyMs = Math.round(performance.now() - start);
		} catch (err: unknown) {
			if (err instanceof Error && err.name === 'AbortError') {
				// Aborted by user
			} else {
				liveError = err instanceof Error ? err.message : 'Chat completion failed';
			}
		} finally {
			isLiveRunning = false;
			abortController = null;
		}
	}

	async function handleSyncUpstream() {
		syncSuccessMsg = null;
		try {
			const updated = await cavemanState.syncUpstream();
			if (updated) {
				syncSuccessMsg = 'Successfully pulled latest rules from upstream GitHub!';
			} else {
				syncSuccessMsg = 'Already up to date with latest upstream commit.';
			}
			setTimeout(() => {
				syncSuccessMsg = null;
			}, 4000);
		} catch {
			// error stored in cavemanState.error
		}
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

	async function handleCopyLiveOutput() {
		try {
			await navigator.clipboard.writeText(liveOutput);
			copiedOutput = true;
			setTimeout(() => (copiedOutput = false), 2000);
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
						<Card.Title class="text-xl">Caveman Voice & Output Optimization</Card.Title>
						<Badge variant={cavemanState.enabled ? 'default' : 'outline'} class="text-[10px]">
							{cavemanState.enabled ? 'Active' : 'Disabled'}
						</Badge>
						{#if cavemanState.upstreamCommit}
							<a
								href={`${cavemanState.upstreamURL}/commit/${cavemanState.upstreamCommit}`}
								target="_blank"
								rel="noreferrer"
								class="inline-flex items-center gap-1 rounded border border-border/60 bg-muted/40 px-2 py-0.5 font-mono text-[10px] text-muted-foreground transition-colors hover:text-foreground"
							>
								<GitCommitIcon class="h-3 w-3" />
								{cavemanState.upstreamCommit}
								<ExternalLinkIcon class="h-2.5 w-2.5" />
							</a>
						{/if}
						<Badge variant="secondary" class="text-[10px]">Auto-Synced</Badge>
					</div>
					<Card.Description>
						Why many token when few do trick. Enforces ultra-concise system instructions to cut
						model response tokens by ~54% to 75% with zero loss of technical accuracy.
					</Card.Description>
				</div>

				<div class="flex items-center gap-3">
					<Button
						variant="outline"
						size="sm"
						onclick={handleSyncUpstream}
						disabled={cavemanState.syncing}
						class="h-9 gap-1.5 text-xs"
					>
						<RefreshCwIcon class={`h-3.5 w-3.5 ${cavemanState.syncing ? 'animate-spin' : ''}`} />
						{cavemanState.syncing ? 'Syncing...' : 'Sync with GitHub'}
					</Button>

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

		{#if syncSuccessMsg}
			<div
				class="mx-6 mb-4 rounded-md border border-emerald-500/20 bg-emerald-500/10 px-3 py-2 text-xs text-emerald-400"
			>
				{syncSuccessMsg}
			</div>
		{/if}

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
					<div class="text-xs font-medium text-muted-foreground">Upstream Sync</div>
					<div
						class="mt-1 truncate text-sm font-semibold"
						title={cavemanState.upstreamCommitMsg || 'Up to date'}
					>
						{cavemanState.upstreamCommit ? `Commit ${cavemanState.upstreamCommit}` : 'Bundled'}
					</div>
					<div class="mt-0.5 truncate text-[11px] text-muted-foreground">
						{formatDateTime(cavemanState.lastSync)}
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

	<!-- Interactive Sandbox / Tester Card with Provider Selection -->
	<Card.Root>
		<Card.Header>
			<div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
				<div>
					<Card.Title class="text-base font-semibold">Interactive Caveman Live Tester</Card.Title>
					<Card.Description>
						Select a provider & model to preview prompt injection or run real live completions.
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
						{#each currentModels as m (m)}
							<option value={m}>{m}</option>
						{/each}
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
						disabled={cavemanState.testing || isLiveRunning || !testPrompt.trim()}
						class="h-8 gap-1.5 text-xs"
					>
						<SparklesIcon class="h-3.5 w-3.5 text-primary" />
						<span>Simulate Payload</span>
					</Button>

					<Button
						size="sm"
						onclick={handleLiveRun}
						disabled={!testPrompt.trim()}
						class="h-8 gap-1.5 text-xs"
						variant={isLiveRunning ? 'destructive' : 'default'}
					>
						{#if isLiveRunning}
							<SquareIcon class="h-3.5 w-3.5 fill-current" />
							<span>Stop Generation</span>
						{:else}
							<PlayIcon class="h-3.5 w-3.5" />
							<span>Run Live Model</span>
						{/if}
					</Button>
				</div>
			</div>

			<!-- Live Model Execution Output -->
			{#if isLiveRunning || liveOutput || liveError}
				<div class="space-y-3 rounded-lg border bg-muted/20 p-4">
					<div class="flex flex-wrap items-center justify-between gap-2 border-b pb-2">
						<div class="flex items-center gap-2">
							{#if selectedProvider === 'antigravity'}
								<RocketIcon class="h-4 w-4 text-amber-500" />
							{:else}
								<BotIcon class="h-4 w-4 text-primary" />
							{/if}
							<span class="text-xs font-semibold">Live Model Output</span>
							<Badge variant="secondary" class="font-mono text-[10px]">
								{selectedModel}
							</Badge>
							<Badge variant="outline" class="font-mono text-[10px]">
								Mode: {cavemanState.mode}
							</Badge>
						</div>

						<div class="flex items-center gap-3 text-xs">
							{#if liveLatencyMs !== null}
								<span class="font-mono text-[11px] text-muted-foreground">
									{liveLatencyMs}ms
								</span>
							{/if}
							{#if liveOutput}
								<span class="font-mono text-[11px] text-emerald-500">
									~{liveOutput.split(/\s+/).filter(Boolean).length} words
								</span>
								<Button
									variant="ghost"
									size="sm"
									class="h-6 px-1.5 text-xs text-muted-foreground hover:text-foreground"
									onclick={handleCopyLiveOutput}
								>
									{#if copiedOutput}
										<CheckIcon class="h-3 w-3 text-emerald-500" />
									{:else}
										<CopyIcon class="h-3 w-3" />
									{/if}
								</Button>
							{/if}
						</div>
					</div>

					{#if liveError}
						<div
							class="flex items-center gap-2 rounded-md border border-destructive/20 bg-destructive/10 p-3 text-xs text-destructive"
						>
							<AlertCircleIcon class="h-4 w-4 shrink-0" />
							<span>{liveError}</span>
						</div>
					{:else}
						<div
							class="max-h-64 overflow-y-auto rounded-md border bg-zinc-950 p-3.5 font-mono text-xs leading-relaxed text-zinc-100"
						>
							<pre class="whitespace-pre-wrap">{liveOutput ||
									(isLiveRunning ? 'Generating response in smart caveman style...' : '')}</pre>
						</div>
					{/if}
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

						<!-- Caveman Voice Output Comparison -->
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
