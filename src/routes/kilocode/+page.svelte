<script lang="ts">
	import { kilocodeState } from '$lib/state/kilocode.svelte';
	import KilocodeProvider from '$lib/components/providers/kilocode-provider.svelte';
</script>

<KilocodeProvider {kilocodeState} />
