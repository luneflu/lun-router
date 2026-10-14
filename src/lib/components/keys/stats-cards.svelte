<script lang="ts">
	import * as Card from '$lib/components/ui/card';
	import KeyRoundIcon from '@lucide/svelte/icons/key-round';
	import CheckCircle2Icon from '@lucide/svelte/icons/check-circle-2';

	interface Props {
		totalCount: number;
		activeCount: number;
	}

	let { totalCount, activeCount }: Props = $props();
</script>

<div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
	<Card.Root>
		<Card.Header class="flex flex-row items-center justify-between pb-2">
			<Card.Title class="text-xs font-medium text-muted-foreground">Total API Keys</Card.Title>
			<div
				class="flex size-8 items-center justify-center rounded-md bg-muted text-muted-foreground"
			>
				<KeyRoundIcon class="size-4" />
			</div>
		</Card.Header>
		<Card.Content>
			<div class="text-2xl font-bold">{totalCount}</div>
			<p class="mt-1 text-xs text-muted-foreground">Managed router credentials</p>
		</Card.Content>
	</Card.Root>

	<Card.Root>
		<Card.Header class="flex flex-row items-center justify-between pb-2">
			<Card.Title class="text-xs font-medium text-muted-foreground">Active Keys</Card.Title>
			<div
				class="flex size-8 items-center justify-center rounded-md bg-emerald-500/10 text-emerald-500"
			>
				<CheckCircle2Icon class="size-4" />
			</div>
		</Card.Header>
		<Card.Content>
			<div class="text-2xl font-bold text-emerald-500">{activeCount}</div>
			<p class="mt-1 text-xs text-muted-foreground">Ready for completions</p>
		</Card.Content>
	</Card.Root>
</div>
