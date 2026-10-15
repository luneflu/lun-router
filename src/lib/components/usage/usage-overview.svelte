<script lang="ts">
	import * as Card from '$lib/components/ui/card';
	import { Button } from '$lib/components/ui/button';
	import ActivityIcon from '@lucide/svelte/icons/activity';
	import ArrowRightIcon from '@lucide/svelte/icons/arrow-right';
	import { usageState } from '$lib/state/usage.svelte';

	function formatTokens(n: number): string {
		if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(2)}M`;
		if (n >= 1_000) return `${(n / 1_000).toFixed(1)}k`;
		return `${n}`;
	}

	const totals = $derived(usageState.summary?.totals);
	const days = $derived(usageState.summary?.by_day ?? []);
	const maxRequests = $derived(Math.max(1, ...days.map((d) => d.requests)));
</script>

<Card.Root>
	<Card.Header class="flex flex-row items-center justify-between pb-2">
		<div>
			<Card.Title class="flex items-center gap-2 text-xs font-medium text-muted-foreground">
				<ActivityIcon class="size-3.5" />
				Usage · last 7 days
			</Card.Title>
		</div>
		<a href="/usage">
			<Button size="sm" variant="ghost" class="h-7 gap-1 px-2 text-xs">
				Details
				<ArrowRightIcon class="size-3.5" />
			</Button>
		</a>
	</Card.Header>
	<Card.Content class="flex flex-col gap-4">
		<div class="grid grid-cols-2 gap-4 sm:grid-cols-4">
			<div>
				<p class="text-[11px] text-muted-foreground">Requests</p>
				<p class="text-xl font-bold">{totals?.requests ?? 0}</p>
			</div>
			<div>
				<p class="text-[11px] text-muted-foreground">Input tokens</p>
				<p class="text-xl font-bold">{formatTokens(totals?.in ?? 0)}</p>
			</div>
			<div>
				<p class="text-[11px] text-muted-foreground">Output tokens</p>
				<p class="text-xl font-bold">{formatTokens(totals?.out ?? 0)}</p>
			</div>
			<div>
				<p class="text-[11px] text-muted-foreground">Failed</p>
				<p class="text-xl font-bold text-amber-500">{totals?.errors ?? 0}</p>
			</div>
		</div>

		{#if days.length > 1}
			<div class="flex h-12 items-end gap-1" aria-hidden="true">
				{#each days as day (day.date)}
					<div
						class={`flex-1 rounded-t-sm ${day.errors > 0 ? 'bg-amber-500/60' : 'bg-primary/50'}`}
						style={`height: ${Math.max(3, (day.requests / maxRequests) * 100)}%`}
						title={`${day.date}: ${day.requests} requests`}
					></div>
				{/each}
			</div>
		{/if}
	</Card.Content>
</Card.Root>
