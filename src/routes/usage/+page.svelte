<script lang="ts">
	import { onMount } from 'svelte';
	import { usageState } from '$lib/state/usage.svelte';
	import UsageDetail from '$lib/components/usage/usage-detail.svelte';

	onMount(() => {
		usageState.load();
	});
</script>

<div class="mb-6 flex flex-col gap-1">
	<h1 class="text-2xl font-bold tracking-tight">Usage</h1>
	<p class="text-sm text-muted-foreground">
		Requests and tokens recorded per API key, provider, model and time bucket.
	</p>
</div>

<UsageDetail />
