<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import CompactorCard from '$lib/components/tokensaver/compactor-card.svelte';
	import CavemanCard from '$lib/components/tokensaver/caveman-card.svelte';
	import PonytailCard from '$lib/components/tokensaver/ponytail-card.svelte';
	import { compactorState } from '$lib/state/compactor.svelte';
	import { cavemanState } from '$lib/state/caveman.svelte';
	import { ponytailState } from '$lib/state/ponytail.svelte';
	import ZapIcon from '@lucide/svelte/icons/zap';
	import BotIcon from '@lucide/svelte/icons/bot';
	import ScissorsIcon from '@lucide/svelte/icons/scissors';
	import { Badge } from '$lib/components/ui/badge';

	let activeTab = $state<'compactor' | 'caveman' | 'ponytail'>('compactor');

	onMount(() => {
		const tabParam = page.url.searchParams.get('tab');
		if (tabParam === 'compactor') activeTab = 'compactor';
		if (tabParam === 'caveman' || tabParam === 'ponytail') activeTab = tabParam;
		compactorState.load();
		cavemanState.load();
		ponytailState.load();
	});

	function selectTab(tab: 'compactor' | 'caveman' | 'ponytail') {
		activeTab = tab;
		const url = new URL(window.location.href);
		url.searchParams.set('tab', tab);
		window.history.replaceState(null, '', url.toString());
	}
</script>

<div class="flex flex-col gap-6">
	<div class="flex flex-col gap-1 sm:flex-row sm:items-center sm:justify-between">
		<div>
			<h1 class="text-2xl font-bold tracking-tight">Token Saver</h1>
			<p class="text-sm text-muted-foreground">
				Multi-engine context & token compression: compact tool outputs with the compactor, cut
				output tokens with Caveman, and eliminate code bloat with Ponytail.
			</p>
		</div>
	</div>

	<!-- Tab Switcher Navigation -->
	<div class="flex border-b border-border">
		<button
			type="button"
			onclick={() => selectTab('compactor')}
			class={`flex items-center gap-2 border-b-2 px-4 py-2.5 text-sm font-medium transition-colors ${
				activeTab === 'compactor'
					? 'border-primary text-foreground'
					: 'border-transparent text-muted-foreground hover:border-border hover:text-foreground'
			}`}
		>
			<ZapIcon class="h-4 w-4 text-amber-500" />
			<span>Compactor (tool outputs)</span>
			<Badge
				variant={compactorState.enabled ? 'default' : 'secondary'}
				class="ml-1 h-4 px-1.5 py-0 text-[10px]"
			>
				{compactorState.enabled ? 'Active' : 'Off'}
			</Badge>
		</button>

		<button
			type="button"
			onclick={() => selectTab('caveman')}
			class={`flex items-center gap-2 border-b-2 px-4 py-2.5 text-sm font-medium transition-colors ${
				activeTab === 'caveman'
					? 'border-primary text-foreground'
					: 'border-transparent text-muted-foreground hover:border-border hover:text-foreground'
			}`}
		>
			<BotIcon class="h-4 w-4 text-primary" />
			<span>Caveman (Voice & Prompts)</span>
			<Badge
				variant={cavemanState.enabled ? 'default' : 'secondary'}
				class="ml-1 h-4 px-1.5 py-0 text-[10px]"
			>
				{cavemanState.enabled ? cavemanState.mode : 'Off'}
			</Badge>
		</button>

		<button
			type="button"
			onclick={() => selectTab('ponytail')}
			class={`flex items-center gap-2 border-b-2 px-4 py-2.5 text-sm font-medium transition-colors ${
				activeTab === 'ponytail'
					? 'border-primary text-foreground'
					: 'border-transparent text-muted-foreground hover:border-border hover:text-foreground'
			}`}
		>
			<ScissorsIcon class="h-4 w-4 text-emerald-500" />
			<span>Ponytail (Lazy Senior Dev)</span>
			<Badge
				variant={ponytailState.enabled ? 'default' : 'secondary'}
				class="ml-1 h-4 px-1.5 py-0 text-[10px]"
			>
				{ponytailState.enabled ? ponytailState.mode : 'Off'}
			</Badge>
		</button>
	</div>

	<!-- Tab Content Panes -->
	{#if activeTab === 'compactor'}
		<CompactorCard />
	{:else if activeTab === 'caveman'}
		<CavemanCard />
	{:else if activeTab === 'ponytail'}
		<PonytailCard />
	{/if}
</div>
