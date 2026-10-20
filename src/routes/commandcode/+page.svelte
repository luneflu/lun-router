<script lang="ts">
	import { commandCodeState } from '$lib/state/commandcode.svelte';
	import CommandCodeProvider from '$lib/components/providers/commandcode-provider.svelte';
</script>

<CommandCodeProvider {commandCodeState} />
