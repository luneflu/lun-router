<script lang="ts">
	import { providerState } from '$lib/state/provider.svelte';
	import OpenCodeProvider from '$lib/components/providers/opencode-provider.svelte';
</script>

<OpenCodeProvider {providerState} />
