<script lang="ts">
	import * as Dialog from '$lib/components/ui/dialog';
	import * as Alert from '$lib/components/ui/alert';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { toast } from '$lib/state/toast.svelte';
	import { createCommandCodeAccount } from '$lib/api/commandcode';

	import AlertCircleIcon from '@lucide/svelte/icons/alert-circle';
	import KeyRoundIcon from '@lucide/svelte/icons/key-round';
	import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';

	interface Props {
		open: boolean;
		onSuccess: () => Promise<void>;
		onClose: () => void;
	}

	let { open = $bindable(false), onSuccess, onClose }: Props = $props();

	let apiKey = $state('');
	let name = $state('');
	let submitting = $state(false);
	let error = $state<string | null>(null);

	async function handleSubmit(e: SubmitEvent) {
		e.preventDefault();
		if (!apiKey.trim()) {
			error = 'API key is required';
			return;
		}

		try {
			submitting = true;
			error = null;
			await createCommandCodeAccount({
				apiKey: apiKey.trim(),
				name: name.trim() || undefined
			});
			toast.success('Account Connected', 'Command Code API key verified');
			await onSuccess();
			apiKey = '';
			name = '';
			open = false;
		} catch (e: unknown) {
			error = e instanceof Error ? e.message : 'Failed to connect API key';
		} finally {
			submitting = false;
		}
	}
</script>

<Dialog.Root bind:open>
	<Dialog.Content class="sm:max-w-lg">
		<Dialog.Header>
			<Dialog.Title class="flex items-center gap-2 text-base font-semibold">
				<KeyRoundIcon class="size-4 text-indigo-500" />
				Add Command Code Key
			</Dialog.Title>
			<Dialog.Description class="text-xs">
				Paste your Command Code CLI API key. It is verified against
				<code class="font-mono">/alpha/whoami</code> before it is saved.
			</Dialog.Description>
		</Dialog.Header>

		{#if error}
			<Alert.Root variant="destructive">
				<AlertCircleIcon class="size-4" />
				<Alert.Title>Error</Alert.Title>
				<Alert.Description class="text-xs">{error}</Alert.Description>
			</Alert.Root>
		{/if}

		<form onsubmit={handleSubmit} class="flex flex-col gap-4">
			<div class="flex flex-col gap-1.5">
				<label for="cc-key" class="text-xs font-medium text-foreground">API Key</label>
				<Input
					id="cc-key"
					type="password"
					bind:value={apiKey}
					placeholder="user_..."
					class="font-mono text-xs"
					required
				/>
				<span class="text-[11px] text-muted-foreground">
					Found in <code class="font-mono">~/.commandcode/auth.json</code> (<code class="font-mono"
						>apiKey</code
					>) or via <code class="font-mono">COMMAND_CODE_API_KEY</code>.
				</span>
			</div>

			<div class="flex flex-col gap-1.5">
				<label for="cc-name" class="text-xs font-medium text-foreground">Label (optional)</label>
				<Input
					id="cc-name"
					type="text"
					bind:value={name}
					placeholder="Personal CLI"
					class="text-xs"
				/>
			</div>

			<Dialog.Footer class="mt-2 flex items-center justify-end gap-2">
				<Button
					type="button"
					variant="ghost"
					size="sm"
					onclick={() => {
						open = false;
						onClose();
					}}
				>
					Cancel
				</Button>
				<Button type="submit" variant="default" size="sm" disabled={submitting}>
					{#if submitting}
						<RefreshCwIcon data-icon="inline-start" class="size-3.5 animate-spin" />
						Verifying...
					{:else}
						Connect Key
					{/if}
				</Button>
			</Dialog.Footer>
		</form>
	</Dialog.Content>
</Dialog.Root>
