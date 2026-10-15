<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import RtkCard from '$lib/components/tokensaver/rtk-card.svelte';
	import CavemanCard from '$lib/components/tokensaver/caveman-card.svelte';
	import { tokenSaverState } from '$lib/state/tokensaver.svelte';
	import { cavemanState } from '$lib/state/caveman.svelte';
	import ZapIcon from '@lucide/svelte/icons/zap';
	import BotIcon from '@lucide/svelte/icons/bot';
	import { Badge } from '$lib/components/ui/badge';

	let activeTab = $state<'rtk' | 'caveman'>('rtk');

	onMount(() => {
		const tabParam = page.url.searchParams.get('tab');
		if (tabParam === 'caveman' || tabParam === 'rtk') {
			activeTab = tabParam;
		}
		tokenSaverState.load();
		cavemanState.load();
	});

	function selectTab(tab: 'rtk' | 'caveman') {
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
				Dual-engine context & token compression: shrink tool outputs with RTK, cut output tokens
				with Caveman.
			</p>
		</div>
	</div>

	<!-- Tab Switcher Navigation -->
	<div class="flex border-b border-border">
		<button
			type="button"
			onclick={() => selectTab('rtk')}
			class={`flex items-center gap-2 border-b-2 px-4 py-2.5 text-sm font-medium transition-colors ${
				activeTab === 'rtk'
					? 'border-primary text-foreground'
					: 'border-transparent text-muted-foreground hover:border-border hover:text-foreground'
			}`}
		>
			<ZapIcon class="h-4 w-4 text-amber-500" />
			<span>RTK (Tool Outputs)</span>
			<Badge
				variant={tokenSaverState.rtkEnabled ? 'default' : 'secondary'}
				class="ml-1 h-4 px-1.5 py-0 text-[10px]"
			>
				{tokenSaverState.rtkEnabled ? 'Active' : 'Off'}
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
	</div>

	<!-- Tab Content Panes -->
	{#if activeTab === 'rtk'}
		<RtkCard />
	{:else if activeTab === 'caveman'}
		<CavemanCard />
	{/if}
</div>
