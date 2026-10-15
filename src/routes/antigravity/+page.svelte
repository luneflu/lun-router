<script lang="ts">
	import { antigravityState } from '$lib/state/antigravity.svelte';
	import AntigravityProvider from '$lib/components/providers/antigravity-provider.svelte';
</script>

<AntigravityProvider {antigravityState} />
