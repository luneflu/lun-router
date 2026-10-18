<script lang="ts">
	import { untrack } from 'svelte';
	import * as Dialog from '$lib/components/ui/dialog';
	import * as Alert from '$lib/components/ui/alert';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { cn } from '$lib/utils';
	import {
		requestKilocodeDeviceCode,
		pollKilocodeToken,
		createKilocodeAccount,
		type DeviceCodeResponse
	} from '$lib/api/kilocode';
	import { toast } from '$lib/state/toast.svelte';

	import ExternalLinkIcon from '@lucide/svelte/icons/external-link';
	import CopyIcon from '@lucide/svelte/icons/copy';
	import CheckIcon from '@lucide/svelte/icons/check';
	import AlertCircleIcon from '@lucide/svelte/icons/alert-circle';
	import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';
	import KeyRoundIcon from '@lucide/svelte/icons/key-round';
	import CodeIcon from '@lucide/svelte/icons/code';

	interface Props {
		open: boolean;
		onSuccess: () => Promise<void>;
		onClose: () => void;
	}

	let { open = $bindable(false), onSuccess, onClose }: Props = $props();

	let tab = $state<'oauth' | 'manual'>('oauth');

	// Device code OAuth state
	let deviceData = $state<DeviceCodeResponse | null>(null);
	let deviceLoading = $state(false);
	let polling = $state(false);
	let pollStatus = $state<'waiting' | 'approved' | 'denied' | 'expired' | 'error'>('waiting');
	let copiedCode = $state(false);
	let copiedUrl = $state(false);
	let pollError = $state<string | null>(null);
	let pollTimer: ReturnType<typeof setInterval> | null = null;
	let secondsLeft = $state(300);
	let countdownTimer: ReturnType<typeof setInterval> | null = null;

	// Manual mode inputs
	let manualToken = $state('');
	let manualEmail = $state('');
	let manualOrgId = $state('');
	let submittingManual = $state(false);
	let manualError = $state<string | null>(null);

	function stopPolling() {
		if (pollTimer) {
			clearInterval(pollTimer);
			pollTimer = null;
		}
		if (countdownTimer) {
			clearInterval(countdownTimer);
			countdownTimer = null;
		}
		polling = false;
	}

	async function startDeviceFlow() {
		stopPolling();
		deviceLoading = true;
		pollError = null;
		pollStatus = 'waiting';

		try {
			const data = await requestKilocodeDeviceCode();
			deviceData = data;
			secondsLeft = data.expires_in || 300;

			// Auto open verification URL
			const verifyUrl = data.verification_uri_complete || data.verification_uri;
			if (verifyUrl && typeof window !== 'undefined') {
				window.open(verifyUrl, '_blank', 'noopener,noreferrer');
			}

			startPolling(data.device_code, data.interval || 3);
		} catch (e: unknown) {
			pollError = e instanceof Error ? e.message : 'Failed to initiate device authorization';
			pollStatus = 'error';
		} finally {
			deviceLoading = false;
		}
	}

	function startPolling(code: string, intervalSeconds: number) {
		polling = true;

		// Countdown timer
		countdownTimer = setInterval(() => {
			if (secondsLeft > 0) {
				secondsLeft--;
			} else {
				stopPolling();
				pollStatus = 'expired';
				pollError = 'Authorization code expired. Please restart the authorization.';
			}
		}, 1000);

		// Polling interval
		pollTimer = setInterval(async () => {
			if (!open || !polling) {
				stopPolling();
				return;
			}

			try {
				const res = await pollKilocodeToken(code);
				if (res.status === 'approved' && res.success) {
					stopPolling();
					pollStatus = 'approved';
					toast.success('Account Connected', `Kilo Code account connected successfully`);
					await onSuccess();
					setTimeout(() => {
						open = false;
					}, 1000);
				} else if (res.status === 'access_denied') {
					stopPolling();
					pollStatus = 'denied';
					pollError = 'Authorization was denied by user.';
				} else if (res.status === 'expired_token') {
					stopPolling();
					pollStatus = 'expired';
					pollError = 'Device code has expired.';
				}
			} catch (e: unknown) {
				console.warn('Poll error:', e);
			}
		}, Math.max(intervalSeconds, 2) * 1000);
	}

	async function handleManualSubmit(e: SubmitEvent) {
		e.preventDefault();
		if (!manualToken.trim()) {
			manualError = 'Access token is required';
			return;
		}

		try {
			submittingManual = true;
			manualError = null;
			await createKilocodeAccount({
				token: manualToken.trim(),
				email: manualEmail.trim() || undefined,
				orgId: manualOrgId.trim() || undefined
			});
			toast.success('Account Connected', 'Kilo Code account added');
			await onSuccess();
			open = false;
		} catch (e: unknown) {
			manualError = e instanceof Error ? e.message : 'Failed to connect token';
		} finally {
			submittingManual = false;
		}
	}

	async function copyToClipboard(text: string, type: 'code' | 'url') {
		try {
			await navigator.clipboard.writeText(text);
			if (type === 'code') {
				copiedCode = true;
				setTimeout(() => (copiedCode = false), 2000);
			} else {
				copiedUrl = true;
				setTimeout(() => (copiedUrl = false), 2000);
			}
		} catch {
			// fallback
		}
	}

	function resetState() {
		stopPolling();
		deviceData = null;
		pollError = null;
		manualError = null;
		manualToken = '';
		manualEmail = '';
		manualOrgId = '';
	}

	$effect(() => {
		if (open && tab === 'oauth') {
			untrack(() => {
				if (!deviceData && !deviceLoading) {
					startDeviceFlow();
				}
			});
		}
	});
</script>

<Dialog.Root
	bind:open
	onOpenChange={(val) => {
		if (!val) {
			resetState();
			onClose();
		}
	}}
>
	<Dialog.Content class="max-w-md gap-0 p-0 sm:max-w-lg">
		<!-- Header -->
		<div class="border-b border-border p-5">
			<div class="flex items-center gap-3">
				<div
					class="flex size-10 items-center justify-center rounded-xl border border-orange-500/20 bg-orange-500/10 text-orange-500"
				>
					<CodeIcon class="size-5" />
				</div>
				<div class="flex flex-col">
					<Dialog.Title class="text-base font-bold">Connect Kilo Code</Dialog.Title>
					<Dialog.Description class="text-xs text-muted-foreground">
						Connect using Kilo Code device authorization or manual API token.
					</Dialog.Description>
				</div>
			</div>

			<!-- Tabs -->
			<div class="mt-4 flex rounded-lg border border-border bg-muted/30 p-1">
				<button
					type="button"
					class={cn(
						'flex flex-1 items-center justify-center gap-2 rounded-md py-1.5 text-xs font-medium transition-colors',
						tab === 'oauth'
							? 'bg-background text-foreground shadow-xs'
							: 'text-muted-foreground hover:text-foreground'
					)}
					onclick={() => {
						tab = 'oauth';
						if (!deviceData && !deviceLoading) startDeviceFlow();
					}}
				>
					<CodeIcon class="size-3.5 text-orange-500" />
					Device Code (OAuth)
				</button>
				<button
					type="button"
					class={cn(
						'flex flex-1 items-center justify-center gap-2 rounded-md py-1.5 text-xs font-medium transition-colors',
						tab === 'manual'
							? 'bg-background text-foreground shadow-xs'
							: 'text-muted-foreground hover:text-foreground'
					)}
					onclick={() => {
						tab = 'manual';
						stopPolling();
					}}
				>
					<KeyRoundIcon class="size-3.5 text-primary" />
					Manual Token
				</button>
			</div>
		</div>

		<!-- Body -->
		<div class="p-5">
			{#if tab === 'oauth'}
				<div class="flex flex-col gap-4">
					{#if deviceLoading}
						<div class="flex flex-col items-center justify-center py-10 gap-3">
							<RefreshCwIcon class="size-8 animate-spin text-orange-500" />
							<span class="text-xs font-medium text-muted-foreground">
								Requesting authorization code from Kilo Code...
							</span>
						</div>
					{:else if deviceData}
						<div class="flex flex-col gap-3 rounded-lg border border-border bg-card p-4">
							<div class="flex items-center justify-between">
								<span class="text-xs font-semibold text-muted-foreground uppercase tracking-wider">
									User Code
								</span>
								<span class="font-mono text-[11px] text-muted-foreground">
									Expires in {Math.floor(secondsLeft / 60)}:{(secondsLeft % 60).toString().padStart(2, '0')}
								</span>
							</div>

							<div class="flex items-center gap-2">
								<div
									class="flex-1 rounded-lg border border-orange-500/30 bg-orange-500/5 px-4 py-2.5 font-mono text-xl font-bold tracking-widest text-center text-orange-500 select-all"
								>
									{deviceData.user_code}
								</div>
								<Button
									variant="outline"
									size="icon"
									class="shrink-0"
									onclick={() => copyToClipboard(deviceData!.user_code, 'code')}
									title="Copy code"
								>
									{#if copiedCode}
										<CheckIcon class="size-4 text-emerald-500" />
									{:else}
										<CopyIcon class="size-4" />
									{/if}
								</Button>
							</div>

							<p class="text-[11px] text-muted-foreground">
								Confirm or paste this code in your browser on the Kilo Code verification page.
							</p>

							<div class="flex flex-col gap-2 pt-2">
								<Button
									variant="default"
									class="w-full gap-2 bg-orange-600 hover:bg-orange-700 text-white font-medium"
									onclick={() => {
										const url = deviceData!.verification_uri_complete || deviceData!.verification_uri;
										window.open(url, '_blank', 'noopener,noreferrer');
									}}
								>
									<ExternalLinkIcon class="size-4" />
									Open Kilo Verification Page
								</Button>

								<Button
									variant="outline"
									size="sm"
									class="gap-1.5 text-xs text-muted-foreground"
									onclick={() => copyToClipboard(deviceData!.verification_uri, 'url')}
								>
									{#if copiedUrl}
										<CheckIcon class="size-3 text-emerald-500" />
										<span>Copied URL!</span>
									{:else}
										<CopyIcon class="size-3" />
										<span>Copy Verification Link</span>
									{/if}
								</Button>
							</div>
						</div>

						<!-- Polling status indicator -->
						<div class="flex items-center justify-between rounded-lg border border-border/60 bg-muted/30 px-3 py-2">
							<div class="flex items-center gap-2">
								{#if pollStatus === 'waiting'}
									<RefreshCwIcon class="size-3.5 animate-spin text-orange-500" />
									<span class="text-xs text-muted-foreground">Waiting for approval in browser...</span>
								{:else if pollStatus === 'approved'}
									<CheckIcon class="size-3.5 text-emerald-500" />
									<span class="text-xs font-semibold text-emerald-500">Approved! Connecting...</span>
								{:else}
									<AlertCircleIcon class="size-3.5 text-destructive" />
									<span class="text-xs text-destructive">{pollError || 'Authorization failed'}</span>
								{/if}
							</div>

							{#if pollStatus !== 'waiting' && pollStatus !== 'approved'}
								<Button
									variant="ghost"
									size="sm"
									class="h-7 text-xs text-primary"
									onclick={startDeviceFlow}
								>
									Restart
								</Button>
							{/if}
						</div>
					{/if}

					{#if pollError && !deviceData}
						<Alert.Root variant="destructive">
							<AlertCircleIcon class="size-4" />
							<Alert.Title>Error</Alert.Title>
							<Alert.Description class="text-xs">{pollError}</Alert.Description>
						</Alert.Root>

						<Button variant="outline" class="w-full mt-2" onclick={startDeviceFlow}>
							Try Again
						</Button>
					{/if}
				</div>
			{:else}
				<!-- Manual Token Form -->
				<form onsubmit={handleManualSubmit} class="flex flex-col gap-4">
					<div class="flex flex-col gap-1.5">
						<label for="manual-token" class="text-xs font-medium text-foreground">
							Kilo Code Access Token <span class="text-destructive">*</span>
						</label>
						<Input
							id="manual-token"
							type="password"
							placeholder="Paste Kilo Code API / Bearer token"
							bind:value={manualToken}
							required
							class="font-mono text-xs"
						/>
						<span class="text-[11px] text-muted-foreground">
							Bearer token from Kilo Code profile or authorization flow.
						</span>
					</div>

					<div class="flex flex-col gap-1.5">
						<label for="manual-org" class="text-xs font-medium text-foreground">
							Organization ID (Optional)
						</label>
						<Input
							id="manual-org"
							type="text"
							placeholder="Auto-detected from profile if empty"
							bind:value={manualOrgId}
							class="font-mono text-xs"
						/>
					</div>

					<div class="flex flex-col gap-1.5">
						<label for="manual-email" class="text-xs font-medium text-foreground">
							Account Email (Optional)
						</label>
						<Input
							id="manual-email"
							type="email"
							placeholder="Auto-detected from profile if empty"
							bind:value={manualEmail}
							class="text-xs"
						/>
					</div>

					{#if manualError}
						<Alert.Root variant="destructive">
							<AlertCircleIcon class="size-4" />
							<Alert.Title>Failed to connect</Alert.Title>
							<Alert.Description class="text-xs">{manualError}</Alert.Description>
						</Alert.Root>
					{/if}

					<div class="flex justify-end gap-2 pt-2">
						<Button variant="outline" type="button" onclick={() => (open = false)}>
							Cancel
						</Button>
						<Button
							type="submit"
							disabled={submittingManual || !manualToken.trim()}
							class="gap-1.5 bg-orange-600 hover:bg-orange-700 text-white"
						>
							{#if submittingManual}
								<RefreshCwIcon class="size-4 animate-spin" />
								<span>Verifying...</span>
							{:else}
								<KeyRoundIcon class="size-4" />
								<span>Connect Account</span>
							{/if}
						</Button>
					</div>
				</form>
			{/if}
		</div>
	</Dialog.Content>
</Dialog.Root>
