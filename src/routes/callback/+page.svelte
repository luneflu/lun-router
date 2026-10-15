<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import * as Card from '$lib/components/ui/card';
	import { Button } from '$lib/components/ui/button';
	import * as Alert from '$lib/components/ui/alert';
	import { createAntigravityAccount } from '$lib/api/antigravity';
	import { toast } from '$lib/state/toast.svelte';

	import CheckCircle2Icon from '@lucide/svelte/icons/check-circle-2';
	import AlertCircleIcon from '@lucide/svelte/icons/alert-circle';
	import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';
	import ArrowLeftIcon from '@lucide/svelte/icons/arrow-left';
	import RocketIcon from '@lucide/svelte/icons/rocket';

	let status = $state<'processing' | 'success' | 'error' | 'manual'>('processing');
	let errorMessage = $state<string | null>(null);
	let authCode = $state('');

	onMount(async () => {
		const search = new URLSearchParams(window.location.search);
		const code = search.get('code');
		const error = search.get('error');
		const errorDesc = search.get('error_description');

		if (error) {
			status = 'error';
			errorMessage = errorDesc || error;
			return;
		}

		if (!code) {
			status = 'manual';
			return;
		}

		authCode = code;
		const callbackData = {
			type: 'antigravity_oauth_callback',
			code,
			url: window.location.href,
			origin: window.location.origin
		};

		// 1. Post to opener if opened in a popup
		let handledByOpener = false;
		if (window.opener && !window.opener.closed) {
			try {
				window.opener.postMessage(callbackData, '*');
				handledByOpener = true;
			} catch (e) {
				console.log('postMessage failed:', e);
			}
		}

		// 2. BroadcastChannel to other tabs
		try {
			const bc = new BroadcastChannel('antigravity_oauth');
			bc.postMessage(callbackData);
			bc.close();
		} catch (e) {
			console.log('BroadcastChannel failed:', e);
		}

		// 3. Fallback: localStorage trigger
		try {
			localStorage.setItem('antigravity_oauth_callback', JSON.stringify(callbackData));
		} catch {
			// localStorage unavailable
		}

		if (handledByOpener) {
			status = 'success';
			setTimeout(() => {
				window.close();
			}, 1200);
			return;
		}

		// If user navigated directly in same tab, complete account connection right here
		try {
			const redirectUri = `${window.location.origin}/callback`;
			await createAntigravityAccount({ code, redirectUri });
			status = 'success';
			toast.success('Account Connected', 'Google Antigravity account added');
			setTimeout(() => {
				goto('/antigravity');
			}, 1500);
		} catch (e: unknown) {
			status = 'error';
			errorMessage = e instanceof Error ? e.message : 'Failed to exchange authorization code';
		}
	});
</script>

<div class="flex min-h-[60vh] items-center justify-center p-4">
	<Card.Root class="w-full max-w-md text-center">
		<Card.Header class="flex flex-col items-center gap-3 p-6">
			{#if status === 'processing'}
				<div
					class="flex size-14 items-center justify-center rounded-2xl border border-amber-500/20 bg-amber-500/10 text-amber-500"
				>
					<RefreshCwIcon class="size-6 animate-spin" />
				</div>
				<div class="flex flex-col gap-1">
					<Card.Title class="text-lg font-bold">Authorizing Antigravity</Card.Title>
					<Card.Description class="text-xs">
						Connecting your Google account with Antigravity...
					</Card.Description>
				</div>
			{:else if status === 'success'}
				<div
					class="flex size-14 items-center justify-center rounded-2xl border border-emerald-500/20 bg-emerald-500/10 text-emerald-500"
				>
					<CheckCircle2Icon class="size-7" />
				</div>
				<div class="flex flex-col gap-1">
					<div class="flex items-center justify-center gap-2">
						<Card.Title class="text-lg font-bold text-emerald-600 dark:text-emerald-400">
							Authorization Successful!
						</Card.Title>
					</div>
					<Card.Description class="text-xs">
						Account connected. Returning to dashboard...
					</Card.Description>
				</div>
			{:else if status === 'error'}
				<div
					class="flex size-14 items-center justify-center rounded-2xl border border-destructive/20 bg-destructive/10 text-destructive"
				>
					<AlertCircleIcon class="size-7" />
				</div>
				<div class="flex flex-col gap-1">
					<Card.Title class="text-lg font-bold text-destructive">Authorization Failed</Card.Title>
					<Card.Description class="text-xs">
						Could not complete the OAuth sign-in flow.
					</Card.Description>
				</div>
			{:else}
				<div
					class="flex size-14 items-center justify-center rounded-2xl border border-muted bg-muted/30 text-muted-foreground"
				>
					<RocketIcon class="size-7" />
				</div>
				<div class="flex flex-col gap-1">
					<Card.Title class="text-lg font-bold">OAuth Callback</Card.Title>
					<Card.Description class="text-xs">
						No authorization code detected in URL.
					</Card.Description>
				</div>
			{/if}
		</Card.Header>

		<Card.Content class="px-6 pt-0 pb-6">
			{#if errorMessage}
				<Alert.Root variant="destructive" class="text-start text-xs">
					<AlertCircleIcon class="size-4" />
					<Alert.Title>Error details</Alert.Title>
					<Alert.Description class="text-xs">{errorMessage}</Alert.Description>
				</Alert.Root>
			{/if}

			{#if status === 'manual' && authCode}
				<div class="mt-2 flex flex-col gap-2 rounded-lg bg-muted p-3 text-start">
					<span class="text-xs font-medium text-foreground">Authorization Code:</span>
					<code class="font-mono text-[11px] break-all text-muted-foreground">{authCode}</code>
				</div>
			{/if}
		</Card.Content>

		<Card.Footer class="flex items-center justify-center border-t border-border p-4">
			<Button variant="outline" size="sm" href="/antigravity" class="gap-1.5 text-xs">
				<ArrowLeftIcon class="size-3.5" />
				Go to Antigravity Provider
			</Button>
		</Card.Footer>
	</Card.Root>
</div>
