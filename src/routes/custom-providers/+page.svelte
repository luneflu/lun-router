<script lang="ts">
	import { onMount } from 'svelte';
	import { customProvidersState } from '$lib/state/custom-providers.svelte';
	import CustomProvidersView from '$lib/components/providers/custom-providers-view.svelte';

	onMount(() => {
		customProvidersState.load();
	});
</script>

<CustomProvidersView />
