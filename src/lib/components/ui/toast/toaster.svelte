<script lang="ts">
	import { toast } from '$lib/state/toast.svelte';
	import CheckCircle2Icon from '@lucide/svelte/icons/check-circle-2';
	import AlertCircleIcon from '@lucide/svelte/icons/alert-circle';
	import InfoIcon from '@lucide/svelte/icons/info';
	import XIcon from '@lucide/svelte/icons/x';
</script>

{#if toast.items.length > 0}
	<div
		class="pointer-events-none fixed end-4 bottom-4 z-50 flex max-w-sm flex-col gap-2"
		aria-live="polite"
	>
		{#each toast.items as item (item.id)}
			<div
				class="pointer-events-auto flex items-start justify-between gap-3 rounded-lg border border-border bg-card p-3 shadow-lg transition-all"
			>
				<div class="flex items-start gap-2.5">
					{#if item.type === 'success'}
						<CheckCircle2Icon class="mt-0.5 size-4 shrink-0 text-emerald-500" />
					{:else if item.type === 'error'}
						<AlertCircleIcon class="mt-0.5 size-4 shrink-0 text-destructive" />
					{:else}
						<InfoIcon class="mt-0.5 size-4 shrink-0 text-primary" />
					{/if}
					<div class="flex flex-col">
						<span class="font-mono text-xs font-semibold text-foreground">{item.title}</span>
						{#if item.description}
							<span class="mt-0.5 text-[11px] break-all text-muted-foreground"
								>{item.description}</span
							>
						{/if}
					</div>
				</div>
				<button
					type="button"
					class="p-0.5 text-muted-foreground hover:text-foreground"
					onclick={() => toast.dismiss(item.id)}
					aria-label="Dismiss notification"
				>
					<XIcon class="size-3.5" />
				</button>
			</div>
		{/each}
	</div>
{/if}
