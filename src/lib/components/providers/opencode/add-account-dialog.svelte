<script lang="ts">
	import { onDestroy } from 'svelte';
	import * as Dialog from '$lib/components/ui/dialog';
	import * as Alert from '$lib/components/ui/alert';
	import { Button } from '$lib/components/ui/button';
	import { toast } from '$lib/state/toast.svelte';
	import {
		pollOpenCodeOAuth,
		startOpenCodeOAuth,
		type OpenCodeDeviceAuthorization
	} from '$lib/api/opencode';

	import AlertCircleIcon from '@lucide/svelte/icons/alert-circle';
	import ExternalLinkIcon from '@lucide/svelte/icons/external-link';
	import KeyRoundIcon from '@lucide/svelte/icons/key-round';
	import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';

	interface Props {
		open: boolean;
		onSuccess: () => Promise<void>;
		onClose: () => void;
	}

	let { open = $bindable(false), onSuccess, onClose }: Props = $props();

	let phase = $state<'idle' | 'starting' | 'waiting' | 'error'>('idle');
	let auth = $state<OpenCodeDeviceAuthorization | null>(null);
	let error = $state<string | null>(null);
	let pollTimer: ReturnType<typeof setTimeout> | null = null;

	function clearPoll() {
		if (pollTimer) {
			clearTimeout(pollTimer);
			pollTimer = null;
		}
	}

	function reset() {
		clearPoll();
		phase = 'idle';
		auth = null;
		error = null;
	}

	async function start() {
		reset();
		phase = 'starting';
		try {
			const res = await startOpenCodeOAuth();
			if (!open) return;
			auth = res;
			phase = 'waiting';
			schedulePoll(res.interval);
		} catch (e: unknown) {
			error = e instanceof Error ? e.message : 'Failed to start OpenCode login';
			phase = 'error';
		}
	}

	function schedulePoll(intervalSeconds: number) {
		clearPoll();
		pollTimer = setTimeout(poll, Math.max(1, intervalSeconds) * 1000);
	}

	async function poll() {
		if (!auth || phase !== 'waiting') return;
		try {
			const res = await pollOpenCodeOAuth(auth.device_code);
			if (res.status === 'approved') {
				toast.success('Account Connected', 'OpenCode Go is ready');
				await onSuccess();
				open = false;
				reset();
				return;
			}
			if (res.status === 'error') {
				error = res.error || 'OpenCode authorization failed';
				phase = 'error';
				return;
			}
		} catch (e: unknown) {
			error = e instanceof Error ? e.message : 'Failed to check authorization';
			phase = 'error';
			return;
		}
		if (auth) schedulePoll(auth.interval);
	}

	function openVerification() {
		if (!auth) return;
		window.open(auth.verification_uri_complete, '_blank', 'noopener,noreferrer');
	}

	async function copyCode() {
		if (!auth) return;
		try {
			await navigator.clipboard.writeText(auth.user_code);
			toast.info('Code Copied', auth.user_code);
		} catch {
			toast.error('Copy Failed', 'Copy the code manually');
		}
	}

	function close() {
		open = false;
		reset();
		onClose();
	}

	$effect(() => {
		if (open && phase === 'idle') {
			start();
		} else if (!open) {
			reset();
		}
	});

	onDestroy(clearPoll);
</script>

<Dialog.Root bind:open>
	<Dialog.Content class="sm:max-w-lg">
		<Dialog.Header>
			<Dialog.Title class="flex items-center gap-2 text-base font-semibold">
				<KeyRoundIcon class="size-4 text-orange-500" />
				Connect OpenCode Account
			</Dialog.Title>
			<Dialog.Description class="text-xs">
				Sign in with your OpenCode Console account. The router keeps the OAuth session and refreshes
				it automatically.
			</Dialog.Description>
		</Dialog.Header>

		{#if error}
			<Alert.Root variant="destructive">
				<AlertCircleIcon class="size-4" />
				<Alert.Title>Error</Alert.Title>
				<Alert.Description class="text-xs">{error}</Alert.Description>
			</Alert.Root>
		{/if}

		{#if phase === 'starting'}
			<div class="flex flex-col items-center justify-center gap-3 py-8">
				<RefreshCwIcon class="size-6 animate-spin text-muted-foreground" />
				<p class="text-xs text-muted-foreground">Starting device authorization...</p>
			</div>
		{:else if phase === 'waiting' && auth}
			<div class="flex flex-col gap-4">
				<div class="rounded-lg border border-border bg-muted/30 p-4">
					<p class="text-xs text-muted-foreground">Enter this code on the OpenCode page:</p>
					<button
						type="button"
						class="mt-2 w-full rounded-md border border-border bg-background px-3 py-2 font-mono text-xl font-bold tracking-widest text-foreground transition-colors hover:bg-muted"
						onclick={copyCode}
						title="Copy code"
					>
						{auth.user_code}
					</button>
					<p class="mt-2 text-[11px] text-muted-foreground">
						Click the code to copy it. The login expires in about {Math.round(auth.expires_in / 60)} minutes.
					</p>
				</div>

				<Button variant="outline" class="w-full gap-2" onclick={openVerification}>
					<ExternalLinkIcon class="size-3.5" />
					Open OpenCode Console
				</Button>

				<div class="flex items-center justify-center gap-2 text-xs text-muted-foreground">
					<RefreshCwIcon class="size-3.5 animate-spin" />
					Waiting for approval...
				</div>
			</div>
		{:else if phase === 'error'}
			<Button variant="outline" class="w-full gap-2" onclick={start}>
				<RefreshCwIcon class="size-3.5" />
				Retry Login
			</Button>
		{/if}

		<Dialog.Footer class="mt-2 flex items-center justify-end gap-2">
			<Button type="button" variant="ghost" size="sm" onclick={close}>Cancel</Button>
		</Dialog.Footer>
	</Dialog.Content>
</Dialog.Root>
