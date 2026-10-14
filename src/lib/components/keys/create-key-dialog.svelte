<script lang="ts">
	import * as Dialog from '$lib/components/ui/dialog';
	import * as Alert from '$lib/components/ui/alert';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { cn } from '$lib/utils';
	import type { CreateKeyPayload } from '$lib/types';

	import KeyRoundIcon from '@lucide/svelte/icons/key-round';
	import ServerIcon from '@lucide/svelte/icons/server';
	import AlertCircleIcon from '@lucide/svelte/icons/alert-circle';

	interface Props {
		open: boolean;
		primaryMachineId: string;
		onCreate: (payload: CreateKeyPayload) => Promise<void>;
		onClose: () => void;
	}

	let { open = $bindable(false), primaryMachineId, onCreate, onClose }: Props = $props();

	let name = $state('');
	let mode = $state<'auto' | 'custom'>('auto');
	let customKey = $state('');
	let creating = $state(false);
	let error = $state<string | null>(null);

	async function handleSubmit(e: SubmitEvent) {
		e.preventDefault();
		if (!name.trim()) {
			error = 'Please provide a name for the key';
			return;
		}
		if (mode === 'custom' && !customKey.trim()) {
			error = 'Custom key string cannot be empty';
			return;
		}

		try {
			creating = true;
			error = null;
			const payload: CreateKeyPayload = { name: name.trim() };
			if (mode === 'custom' && customKey.trim()) {
				payload.key = customKey.trim();
			}
			await onCreate(payload);
			name = '';
			customKey = '';
			mode = 'auto';
			open = false;
		} catch (e: unknown) {
			error = e instanceof Error ? e.message : 'Error creating key';
		} finally {
			creating = false;
		}
	}
</script>

<Dialog.Root bind:open>
	<Dialog.Content class="sm:max-w-md">
		<Dialog.Header>
			<Dialog.Title class="flex items-center gap-2 text-base font-semibold">
				<KeyRoundIcon class="size-4 text-primary" />
				Create API Key
			</Dialog.Title>
			<Dialog.Description class="text-xs">
				Generate a new router key or assign your custom key string.
			</Dialog.Description>
		</Dialog.Header>

		<form onsubmit={handleSubmit} class="mt-2 flex flex-col gap-4">
			{#if error}
				<Alert.Root variant="destructive" class="px-3 py-2 text-xs">
					<AlertCircleIcon class="size-4" />
					<Alert.Description>{error}</Alert.Description>
				</Alert.Root>
			{/if}

			<div class="flex flex-col gap-1.5">
				<label for="create-key-name" class="text-xs font-medium text-foreground">
					Key Name / Label <span class="text-destructive">*</span>
				</label>
				<Input
					id="create-key-name"
					type="text"
					bind:value={name}
					placeholder="e.g. Production Client, Cursor IDE, Claude CLI"
					required
				/>
				<span class="text-[11px] text-muted-foreground">Human-friendly name to identify usage.</span
				>
			</div>

			<div class="flex flex-col gap-1.5">
				<span class="text-xs font-medium text-foreground">Key Generation Strategy</span>
				<div class="grid grid-cols-2 gap-1 rounded-lg border border-border bg-muted p-1">
					<button
						type="button"
						class={cn(
							'rounded-md px-3 py-1.5 text-xs font-medium transition-all',
							mode === 'auto'
								? 'bg-background text-foreground shadow-xs'
								: 'text-muted-foreground hover:text-foreground'
						)}
						onclick={() => (mode = 'auto')}
					>
						Auto-generate
					</button>
					<button
						type="button"
						class={cn(
							'rounded-md px-3 py-1.5 text-xs font-medium transition-all',
							mode === 'custom'
								? 'bg-background text-foreground shadow-xs'
								: 'text-muted-foreground hover:text-foreground'
						)}
						onclick={() => (mode = 'custom')}
					>
						Custom Key
					</button>
				</div>
			</div>

			{#if mode === 'auto'}
				<Alert.Root class="border-border bg-muted/40 px-3 py-2.5 text-xs">
					<ServerIcon class="size-4" />
					<Alert.Title class="text-xs font-medium">9router Format Specification</Alert.Title>
					<Alert.Description class="mt-0.5 font-mono text-[11px] text-muted-foreground">
						sk-{primaryMachineId.slice(0, 6)}...-[keyId]-[crc8]
					</Alert.Description>
				</Alert.Root>
			{:else}
				<div class="flex flex-col gap-1.5">
					<label for="create-custom-key" class="text-xs font-medium text-foreground">
						Custom API Key Value <span class="text-destructive">*</span>
					</label>
					<Input
						id="create-custom-key"
						type="text"
						bind:value={customKey}
						placeholder="sk-my-custom-endpoint-key"
						class="font-mono text-xs"
						required
					/>
					<span class="text-[11px] text-muted-foreground">
						Enter any custom token or pre-shared key you want to use.
					</span>
				</div>
			{/if}

			<Dialog.Footer class="gap-2 pt-2">
				<Button type="button" variant="ghost" size="sm" onclick={onClose}>Cancel</Button>
				<Button type="submit" variant="default" size="sm" disabled={creating}>
					{creating ? 'Creating...' : 'Create API Key'}
				</Button>
			</Dialog.Footer>
		</form>
	</Dialog.Content>
</Dialog.Root>
