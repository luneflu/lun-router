<script lang="ts">
	import * as Dialog from '$lib/components/ui/dialog';
	import * as Alert from '$lib/components/ui/alert';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import type { APIKey } from '$lib/types';

	import CheckCircle2Icon from '@lucide/svelte/icons/check-circle-2';
	import AlertCircleIcon from '@lucide/svelte/icons/alert-circle';
	import CopyIcon from '@lucide/svelte/icons/copy';
	import CheckIcon from '@lucide/svelte/icons/check';

	interface Props {
		open: boolean;
		keyData: APIKey | null;
		onClose: () => void;
	}

	let { open = $bindable(false), keyData, onClose }: Props = $props();
	let copied = $state(false);

	async function copyKey() {
		if (!keyData?.key) return;
		try {
			await navigator.clipboard.writeText(keyData.key);
			copied = true;
			setTimeout(() => {
				copied = false;
			}, 2000);
		} catch (e: unknown) {
			console.error('Copy failed', e);
		}
	}
</script>

<Dialog.Root bind:open>
	<Dialog.Content class="sm:max-w-md">
		<Dialog.Header>
			<Dialog.Title class="flex items-center gap-2 text-base font-semibold text-emerald-500">
				<CheckCircle2Icon class="size-4" />
				API Key Created
			</Dialog.Title>
			<Dialog.Description class="text-xs">
				Your key for <strong>{keyData?.name}</strong> is ready to use.
			</Dialog.Description>
		</Dialog.Header>

		<div class="mt-2 flex flex-col gap-4">
			<Alert.Root class="border-amber-500/20 bg-amber-500/10 px-3 py-2.5 text-xs text-amber-500">
				<AlertCircleIcon class="size-4" />
				<Alert.Title class="text-xs font-semibold">Copy your key now</Alert.Title>
				<Alert.Description class="text-[11px] text-amber-600 dark:text-amber-400">
					This key is shown in full only once. It will be masked in the dashboard.
				</Alert.Description>
			</Alert.Root>

			<div class="flex flex-col gap-1.5">
				<span class="text-xs font-medium text-foreground">Generated Key Value</span>
				<div class="flex items-center gap-2">
					<Input
						type="text"
						readonly
						value={keyData?.key || ''}
						class="bg-muted/60 font-mono text-xs select-all"
					/>
					<Button variant="secondary" size="sm" class="shrink-0" onclick={copyKey}>
						{#if copied}
							<CheckIcon data-icon="inline-start" class="text-emerald-500" />
							Copied!
						{:else}
							<CopyIcon data-icon="inline-start" />
							Copy
						{/if}
					</Button>
				</div>
			</div>

			<Dialog.Footer class="pt-2">
				<Button variant="default" size="sm" onclick={onClose}>Done</Button>
			</Dialog.Footer>
		</div>
	</Dialog.Content>
</Dialog.Root>
