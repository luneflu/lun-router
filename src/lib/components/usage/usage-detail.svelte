<script lang="ts">
	import * as Card from '$lib/components/ui/card';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';
	import * as Select from '$lib/components/ui/select';
	import { usageState } from '$lib/state/usage.svelte';

	const rangeLabels: Record<number, string> = { 1: '24h', 7: '7d', 30: '30d', 90: '90d' };

	function formatTokens(n: number): string {
		if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(2)}M`;
		if (n >= 1_000) return `${(n / 1_000).toFixed(1)}k`;
		return `${n}`;
	}

	function formatBytes(n: number): string {
		if (n >= 1_048_576) return `${(n / 1_048_576).toFixed(1)} MB`;
		if (n >= 1024) return `${(n / 1024).toFixed(1)} KB`;
		return `${n} B`;
	}

	function formatTime(ts: string): string {
		if (!ts) return '—';
		return new Date(ts).toLocaleString(undefined, {
			month: 'short',
			day: 'numeric',
			hour: '2-digit',
			minute: '2-digit'
		});
	}

	const totals = $derived(usageState.summary?.totals);
	const hourly = $derived(usageState.summary?.by_hour ?? []);
	const daily = $derived(usageState.summary?.by_day ?? []);
	const buckets = $derived(
		hourly.length > 0
			? hourly.map((h) => ({
					label: `${String(h.hour).padStart(2, '0')}:00`,
					requests: h.requests,
					errors: h.errors,
					in: h.in,
					out: h.out
				}))
			: daily.map((d) => ({
					label: d.date,
					requests: d.requests,
					errors: d.errors,
					in: d.in,
					out: d.out
				}))
	);
	const maxRequests = $derived(Math.max(1, ...buckets.map((b) => b.requests)));
	const bucketLabel = $derived(hourly.length > 0 ? 'Requests per hour (UTC)' : 'Requests per day');
	const keyChoices = $derived(usageState.keyOptions.filter((k) => k.key_id));
	const modelChoices = $derived([...new Set(usageState.modelOptions.map((m) => m.model))]);
</script>

<Card.Root>
	<Card.Header class="flex flex-row flex-wrap items-center justify-between gap-2 pb-3">
		<div>
			<Card.Title class="text-base font-semibold">Usage</Card.Title>
			<Card.Description class="text-xs">
				Token usage by time, provider, API key and model. Dates and hours are UTC.
			</Card.Description>
		</div>
		<div class="flex items-center gap-2">
			{#each usageState.ranges as days (days)}
				<Button
					size="sm"
					variant={usageState.days === days ? 'default' : 'outline'}
					class="h-7 px-2 text-xs"
					onclick={() => usageState.load(days)}
				>
					{rangeLabels[days] ?? `${days}d`}
				</Button>
			{/each}
			<Button
				size="sm"
				variant="ghost"
				class="h-7 px-2"
				disabled={usageState.loading}
				onclick={() => usageState.load()}
			>
				<RefreshCwIcon class={`size-3.5 ${usageState.loading ? 'animate-spin' : ''}`} />
			</Button>
		</div>
	</Card.Header>

	<Card.Content class="flex flex-col gap-5">
		{#if usageState.error}
			<p class="text-xs text-destructive">{usageState.error}</p>
		{/if}

		<!-- Filters -->
		<div class="flex flex-wrap items-end gap-3">
			<div class="flex flex-col gap-1">
				<span class="text-[11px] text-muted-foreground">Provider</span>
				<Select.Root
					type="single"
					value={usageState.prov || 'all'}
					onValueChange={(v) => {
						if (!v) return;
						usageState.prov = v === 'all' ? '' : v;
						usageState.applyFilters();
					}}
				>
					<Select.Trigger class="h-8 w-36 text-xs">
						<Select.Value placeholder="All providers" />
					</Select.Trigger>
					<Select.Content>
						<Select.Item value="all" label="All providers">All providers</Select.Item>
						<Select.Item value="opencode" label="opencode">opencode</Select.Item>
						<Select.Item value="antigravity" label="antigravity">antigravity</Select.Item>
					</Select.Content>
				</Select.Root>
			</div>

			<div class="flex flex-col gap-1">
				<span class="text-[11px] text-muted-foreground">API key</span>
				<Select.Root
					type="single"
					value={usageState.keyId || 'all'}
					onValueChange={(v) => {
						if (!v) return;
						usageState.keyId = v === 'all' ? '' : v;
						usageState.applyFilters();
					}}
				>
					<Select.Trigger class="h-8 w-48 text-xs">
						<Select.Value placeholder="All keys" />
					</Select.Trigger>
					<Select.Content>
						<Select.Item value="all" label="All keys">All keys</Select.Item>
						{#each keyChoices as key (key.key_id)}
							<Select.Item value={key.key_id} label={key.key_name || key.key_id}>
								{key.key_name || key.key_id}
							</Select.Item>
						{/each}
					</Select.Content>
				</Select.Root>
			</div>

			<div class="flex flex-col gap-1">
				<span class="text-[11px] text-muted-foreground">Model</span>
				<Select.Root
					type="single"
					value={usageState.model || 'all'}
					onValueChange={(v) => {
						if (!v) return;
						usageState.model = v === 'all' ? '' : v;
						usageState.applyFilters();
					}}
				>
					<Select.Trigger class="h-8 w-52 text-xs">
						<Select.Value placeholder="All models" />
					</Select.Trigger>
					<Select.Content>
						<Select.Item value="all" label="All models">All models</Select.Item>
						{#each modelChoices as model (model)}
							<Select.Item value={model} label={model}>{model}</Select.Item>
						{/each}
					</Select.Content>
				</Select.Root>
			</div>

			{#if usageState.filtered}
				<Button
					size="sm"
					variant="outline"
					class="h-8 text-xs"
					onclick={() => usageState.resetFilters()}
				>
					Clear filters
				</Button>
			{/if}
		</div>

		<!-- Totals -->
		<div class="grid grid-cols-2 gap-3 sm:grid-cols-4">
			<div class="rounded-md border border-border/60 bg-muted/30 p-3">
				<p class="text-[11px] text-muted-foreground">Requests</p>
				<p class="text-xl font-semibold">{totals?.requests ?? 0}</p>
				<p class="text-[11px] text-muted-foreground">
					{totals?.errors ?? 0} failed · avg {totals?.avg_ms ?? 0} ms
				</p>
			</div>
			<div class="rounded-md border border-border/60 bg-muted/30 p-3">
				<p class="text-[11px] text-muted-foreground">Input tokens</p>
				<p class="text-xl font-semibold">{formatTokens(totals?.in ?? 0)}</p>
				<p class="text-[11px] text-muted-foreground">{formatTokens(totals?.cached ?? 0)} cached</p>
			</div>
			<div class="rounded-md border border-border/60 bg-muted/30 p-3">
				<p class="text-[11px] text-muted-foreground">Output tokens</p>
				<p class="text-xl font-semibold">{formatTokens(totals?.out ?? 0)}</p>
				<p class="text-[11px] text-muted-foreground">
					{formatTokens(totals?.reasoning ?? 0)} reasoning
				</p>
			</div>
			<div class="rounded-md border border-border/60 bg-muted/30 p-3">
				<p class="text-[11px] text-muted-foreground">Total tokens</p>
				<p class="text-xl font-semibold">
					{formatTokens((totals?.in ?? 0) + (totals?.out ?? 0))}
				</p>
				<p class="text-[11px] text-muted-foreground">
					{formatBytes(totals?.saved_bytes ?? 0)} saved by compactor
				</p>
			</div>
		</div>

		<!-- Time buckets -->
		{#if buckets.length > 1}
			<div>
				<p class="mb-2 text-xs font-medium text-muted-foreground">{bucketLabel}</p>
				<div class="flex h-28 items-end gap-1">
					{#each buckets as bucket (bucket.label)}
						<div
							class="flex h-full flex-1 flex-col justify-end"
							title={`${bucket.label}: ${bucket.requests} requests, ${formatTokens(
								bucket.in
							)} in / ${formatTokens(bucket.out)} out`}
						>
							<div
								class={`w-full rounded-t-sm ${
									bucket.errors > 0 ? 'bg-amber-500/70' : 'bg-primary/60'
								}`}
								style={`height: ${Math.max(2, (bucket.requests / maxRequests) * 100)}%`}
							></div>
						</div>
					{/each}
				</div>
				<div class="mt-1 flex justify-between text-[10px] text-muted-foreground">
					<span>{buckets[0]?.label}</span>
					<span>{buckets[buckets.length - 1]?.label}</span>
				</div>
			</div>
		{/if}

		<!-- Providers -->
		{#if usageState.summary && usageState.summary.by_provider.length > 0}
			<div>
				<p class="mb-2 text-xs font-medium text-muted-foreground">By provider</p>
				<div class="grid gap-3 sm:grid-cols-2">
					{#each usageState.summary.by_provider as row (row.prov)}
						<div class="rounded-md border border-border/60 p-3">
							<div class="flex items-center justify-between">
								<Badge variant="secondary" class="text-[10px]">{row.prov}</Badge>
								<span class="text-xs text-muted-foreground">{row.requests} requests</span>
							</div>
							<div class="mt-2 flex gap-4 text-xs">
								<span>{formatTokens(row.in)} in</span>
								<span>{formatTokens(row.out)} out</span>
								{#if row.errors > 0}
									<span class="text-amber-500">{row.errors} failed</span>
								{/if}
							</div>
						</div>
					{/each}
				</div>
			</div>
		{/if}

		<!-- Keys -->
		{#if usageState.summary && usageState.summary.by_key.length > 0}
			<div>
				<p class="mb-2 text-xs font-medium text-muted-foreground">By API key</p>
				<div class="overflow-x-auto rounded-md border border-border/60">
					<table class="w-full text-xs">
						<thead class="bg-muted/40 text-muted-foreground">
							<tr>
								<th class="px-3 py-2 text-left font-medium">Key</th>
								<th class="px-3 py-2 text-right font-medium">Requests</th>
								<th class="px-3 py-2 text-right font-medium">Input</th>
								<th class="px-3 py-2 text-right font-medium">Output</th>
								<th class="px-3 py-2 text-right font-medium">Last used</th>
							</tr>
						</thead>
						<tbody>
							{#each usageState.summary.by_key as row (row.key_id || row.key_name)}
								<tr class="border-t border-border/60">
									<td class="px-3 py-2">
										<span class="font-medium">{row.key_name || 'unnamed'}</span>
										{#if row.errors > 0}
											<span class="ml-2 text-amber-500">{row.errors} failed</span>
										{/if}
									</td>
									<td class="px-3 py-2 text-right">{row.requests}</td>
									<td class="px-3 py-2 text-right">{formatTokens(row.in)}</td>
									<td class="px-3 py-2 text-right">{formatTokens(row.out)}</td>
									<td class="px-3 py-2 text-right text-muted-foreground">
										{formatTime(row.last_ts)}
									</td>
								</tr>
							{/each}
						</tbody>
					</table>
				</div>
			</div>
		{/if}

		<!-- Models -->
		{#if usageState.summary && usageState.summary.by_model.length > 0}
			<div>
				<p class="mb-2 text-xs font-medium text-muted-foreground">By model</p>
				<div class="overflow-x-auto rounded-md border border-border/60">
					<table class="w-full text-xs">
						<thead class="bg-muted/40 text-muted-foreground">
							<tr>
								<th class="px-3 py-2 text-left font-medium">Model</th>
								<th class="px-3 py-2 text-left font-medium">Provider</th>
								<th class="px-3 py-2 text-right font-medium">Requests</th>
								<th class="px-3 py-2 text-right font-medium">Input</th>
								<th class="px-3 py-2 text-right font-medium">Output</th>
							</tr>
						</thead>
						<tbody>
							{#each usageState.summary.by_model as row (`${row.prov}/${row.model}`)}
								<tr class="border-t border-border/60">
									<td class="px-3 py-2 font-medium">{row.model}</td>
									<td class="px-3 py-2">
										<Badge variant="secondary" class="text-[10px]">{row.prov}</Badge>
									</td>
									<td class="px-3 py-2 text-right">
										{row.requests}{#if row.errors > 0}<span class="ml-1 text-amber-500"
												>({row.errors})</span
											>{/if}
									</td>
									<td class="px-3 py-2 text-right">{formatTokens(row.in)}</td>
									<td class="px-3 py-2 text-right">{formatTokens(row.out)}</td>
								</tr>
							{/each}
						</tbody>
					</table>
				</div>
			</div>
		{/if}

		<!-- Recent -->
		{#if usageState.summary && usageState.summary.recent.length > 0}
			<div>
				<p class="mb-2 text-xs font-medium text-muted-foreground">Recent requests</p>
				<div class="overflow-x-auto rounded-md border border-border/60">
					<table class="w-full text-xs">
						<thead class="bg-muted/40 text-muted-foreground">
							<tr>
								<th class="px-3 py-2 text-left font-medium">Time</th>
								<th class="px-3 py-2 text-left font-medium">Key</th>
								<th class="px-3 py-2 text-left font-medium">Model</th>
								<th class="px-3 py-2 text-right font-medium">Tokens</th>
								<th class="px-3 py-2 text-right font-medium">Latency</th>
								<th class="px-3 py-2 text-right font-medium">Status</th>
							</tr>
						</thead>
						<tbody>
							{#each usageState.summary.recent as row (`${row.ts}-${row.model}-${row.status}-${row.ms}`)}
								<tr class="border-t border-border/60">
									<td class="px-3 py-2 text-muted-foreground">{formatTime(row.ts)}</td>
									<td class="px-3 py-2">{row.key_name || '—'}</td>
									<td class="px-3 py-2">
										{row.model}
										{#if row.acct}<span class="ml-1 text-muted-foreground"
												>({row.acct.split('@')[0]})</span
											>{/if}
									</td>
									<td class="px-3 py-2 text-right">
										{formatTokens(row.in)} / {formatTokens(row.out)}
										{#if row.tok_src !== 'upstream'}
											<span class="ml-1 text-muted-foreground" title="provider did not report usage"
												>?</span
											>
										{/if}
									</td>
									<td class="px-3 py-2 text-right">{row.ms} ms</td>
									<td class="px-3 py-2 text-right">
										<span class={row.status >= 400 ? 'text-destructive' : 'text-emerald-500'}>
											{row.status}
										</span>
									</td>
								</tr>
							{/each}
						</tbody>
					</table>
				</div>
			</div>
		{/if}

		{#if !usageState.loading && usageState.summary?.totals.requests === 0}
			<p class="text-xs text-muted-foreground">
				No requests recorded in this window{#if usageState.filtered}
					for these filters{/if}. Usage is written to
				<code class="text-[11px]">data/usage/&lt;date&gt;.jsonl</code>.
			</p>
		{/if}
	</Card.Content>
</Card.Root>
