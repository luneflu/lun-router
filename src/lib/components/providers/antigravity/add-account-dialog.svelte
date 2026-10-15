<script lang="ts">
	import * as Dialog from '$lib/components/ui/dialog';
	import * as Alert from '$lib/components/ui/alert';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { cn } from '$lib/utils';
	import { getAntigravityOAuthUrl, buildClientOAuthUrl } from '$lib/api/antigravity';
	import type { CreateAccountPayload } from '$lib/api/antigravity';

	import ExternalLinkIcon from '@lucide/svelte/icons/external-link';
	import CopyIcon from '@lucide/svelte/icons/copy';
	import CheckIcon from '@lucide/svelte/icons/check';
	import AlertCircleIcon from '@lucide/svelte/icons/alert-circle';
	import ShieldCheckIcon from '@lucide/svelte/icons/shield-check';
	import KeyRoundIcon from '@lucide/svelte/icons/key-round';
	import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';

	interface Props {
		open: boolean;
		onAdd: (payload: CreateAccountPayload) => Promise<void>;
		onClose: () => void;
	}

	let { open = $bindable(false), onAdd, onClose }: Props = $props();

	let tab = $state<'oauth' | 'manual'>('oauth');
	let authUrl = $state(buildClientOAuthUrl());
	let authUrlLoading = $state(false);
	let copied = $state(false);

	// OAuth mode inputs
	let authCodeOrUrl = $state('');

	// Manual mode inputs
	let email = $state('');
	let projectId = $state('');
	let tokenOrJson = $state('');

	let submitting = $state(false);
	let error = $state<string | null>(null);
	// Resolved by the backend and always on loopback: Google only has a loopback
	// URI registered for this client ID, so a public domain is rejected. The
	// exchange reuses whichever URI the authorize request used.
	let oauthRedirectUri = $state<string | null>(null);

	$effect(() => {
		if (open && tab === 'oauth') {
			loadOAuthUrl();
		}

		if (!open) return;

		const handleCallbackData = async (code: string, url?: string) => {
			if (!code || submitting) return;
			authCodeOrUrl = code;
			try {
				submitting = true;
				error = null;
				const redirectUri = url
					? `${new URL(url).origin}/callback`
					: (oauthRedirectUri ?? undefined);
				await onAdd({ code, redirectUri });
				open = false;
			} catch (e: unknown) {
				error = e instanceof Error ? e.message : 'Error completing authorization';
			} finally {
				submitting = false;
			}
		};

		const messageHandler = (ev: MessageEvent) => {
			if (ev.data?.type === 'antigravity_oauth_callback' && ev.data?.code) {
				handleCallbackData(ev.data.code, ev.data.url);
			}
		};

		let bc: BroadcastChannel | null = null;
		try {
			bc = new BroadcastChannel('antigravity_oauth');
			bc.onmessage = (ev) => {
				if (ev.data?.type === 'antigravity_oauth_callback' && ev.data?.code) {
					handleCallbackData(ev.data.code, ev.data.url);
				}
			};
		} catch {
			// BroadcastChannel unavailable
		}

		window.addEventListener('message', messageHandler);

		return () => {
			window.removeEventListener('message', messageHandler);
			bc?.close();
		};
	});

	async function loadOAuthUrl() {
		try {
			authUrlLoading = true;
			const res = await getAntigravityOAuthUrl();
			oauthRedirectUri = res.redirectUri ?? null;
			if (res.authUrl) {
				authUrl = res.authUrl;
			}
		} catch {
			authUrl = buildClientOAuthUrl();
		} finally {
			authUrlLoading = false;
		}
	}

	async function handleCopyUrl() {
		if (!authUrl) return;
		try {
			await navigator.clipboard.writeText(authUrl);
			copied = true;
			setTimeout(() => (copied = false), 2000);
		} catch {
			// Clipboard fallback
		}
	}

	async function handleSubmit(e: SubmitEvent) {
		e.preventDefault();
		error = null;

		try {
			submitting = true;

			if (tab === 'oauth') {
				const trimmed = authCodeOrUrl.trim();
				if (!trimmed) {
					error = 'Please paste the authorization code or full callback URL';
					return;
				}
				await onAdd({ code: trimmed, redirectUri: oauthRedirectUri ?? undefined });
			} else {
				const raw = tokenOrJson.trim();
				if (!raw) {
					error = 'Please provide a refresh token, access token, or connection JSON';
					return;
				}

				// Check if user pasted a JSON blob
				if (raw.startsWith('{') && raw.endsWith('}')) {
					try {
						const parsed = JSON.parse(raw);
						const payload: CreateAccountPayload = {
							email: parsed.email || email.trim() || undefined,
							projectId: parsed.projectId || parsed.project_id || projectId.trim() || undefined,
							refreshToken: parsed.refreshToken || parsed.refresh_token || undefined,
							accessToken: parsed.accessToken || parsed.access_token || undefined
						};
						await onAdd(payload);
					} catch (jsonErr: unknown) {
						error = 'Invalid JSON: ' + (jsonErr instanceof Error ? jsonErr.message : '');
						return;
					}
				} else {
					// Plain token string
					const payload: CreateAccountPayload = {
						email: email.trim() || undefined,
						projectId: projectId.trim() || undefined,
						refreshToken: raw
					};
					await onAdd(payload);
				}
			}

			// Reset form & close
			authCodeOrUrl = '';
			email = '';
			projectId = '';
			tokenOrJson = '';
			open = false;
		} catch (e: unknown) {
			error = e instanceof Error ? e.message : 'Error adding account';
		} finally {
			submitting = false;
		}
	}
</script>

<Dialog.Root bind:open>
	<Dialog.Content class="sm:max-w-lg">
		<Dialog.Header>
			<Dialog.Title class="flex items-center gap-2 text-base font-semibold">
				<ShieldCheckIcon class="size-4 text-amber-500" />
				Add Antigravity Account
			</Dialog.Title>
			<Dialog.Description class="text-xs">
				Connect your Google account to access Gemini 3.8 and Claude models through Antigravity.
			</Dialog.Description>
		</Dialog.Header>

		<!-- Mode Tabs -->
		<div class="grid grid-cols-2 gap-1 rounded-lg bg-muted p-1 text-xs">
			<button
				type="button"
				class={cn(
					'flex items-center justify-center gap-1.5 rounded-md py-1.5 font-medium transition-colors',
					tab === 'oauth'
						? 'bg-background text-foreground shadow-xs'
						: 'text-muted-foreground hover:text-foreground'
				)}
				onclick={() => {
					tab = 'oauth';
					error = null;
				}}
			>
				<ShieldCheckIcon class="size-3.5" />
				Google OAuth
			</button>
			<button
				type="button"
				class={cn(
					'flex items-center justify-center gap-1.5 rounded-md py-1.5 font-medium transition-colors',
					tab === 'manual'
						? 'bg-background text-foreground shadow-xs'
						: 'text-muted-foreground hover:text-foreground'
				)}
				onclick={() => {
					tab = 'manual';
					error = null;
				}}
			>
				<KeyRoundIcon class="size-3.5" />
				Manual / JSON
			</button>
		</div>

		{#if error}
			<Alert.Root variant="destructive">
				<AlertCircleIcon class="size-4" />
				<Alert.Title>Error</Alert.Title>
				<Alert.Description class="text-xs">{error}</Alert.Description>
			</Alert.Root>
		{/if}

		<form onsubmit={handleSubmit} class="flex flex-col gap-4">
			{#if tab === 'oauth'}
				<!-- OAuth Flow -->
				<div class="flex flex-col gap-3 rounded-lg border border-border bg-card p-3.5 text-xs">
					<div class="flex flex-col gap-1">
						<span class="font-medium text-foreground">1. Authorize via Google</span>
						<p class="text-[11px] text-muted-foreground">
							Click below to open the official Google OAuth consent screen for Antigravity IDE:
						</p>
					</div>

					<div class="flex flex-wrap items-center gap-2">
						{#if authUrl}
							<Button
								variant="default"
								size="sm"
								href={authUrl}
								target="_blank"
								rel="noopener noreferrer"
								disabled={authUrlLoading}
								class="gap-1.5 text-xs"
							>
								<ExternalLinkIcon class="size-3.5" />
								Open Google Sign-in
							</Button>
							<Button
								type="button"
								variant="outline"
								size="sm"
								onclick={handleCopyUrl}
								class="gap-1.5 text-xs"
							>
								{#if copied}
									<CheckIcon class="size-3.5 text-emerald-500" />
									Copied Link
								{:else}
									<CopyIcon class="size-3.5" />
									Copy Link
								{/if}
							</Button>
						{:else}
							<Button variant="outline" size="sm" disabled class="gap-1.5 text-xs">
								<RefreshCwIcon class="size-3.5 animate-spin" />
								Generating Link...
							</Button>
						{/if}
					</div>

					<div class="mt-1 flex flex-col gap-1 border-t border-border pt-2.5">
						<span class="font-medium text-foreground">2. Paste Code or Callback URL</span>
						<p class="text-[11px] text-muted-foreground">
							After authorizing, copy the whole URL from the address bar and paste it here. The
							localhost page will fail to load on a remote server — that is expected:
						</p>
						<Input
							type="text"
							bind:value={authCodeOrUrl}
							placeholder="http://localhost:20128/callback?code=4/0A... or code string"
							class="mt-1 font-mono text-xs"
							required
						/>
					</div>
				</div>
			{:else}
				<!-- Manual Credentials / JSON -->
				<div class="flex flex-col gap-3">
					<div class="flex flex-col gap-1.5">
						<label for="acc-email" class="text-xs font-medium text-foreground">
							Account Email (optional)
						</label>
						<Input
							id="acc-email"
							type="email"
							bind:value={email}
							placeholder="user@gmail.com"
							class="text-xs"
						/>
					</div>

					<div class="flex flex-col gap-1.5">
						<label for="acc-project" class="text-xs font-medium text-foreground">
							Project ID (optional)
						</label>
						<Input
							id="acc-project"
							type="text"
							bind:value={projectId}
							placeholder="cloudaicompanion-xxx (auto-derived if empty)"
							class="font-mono text-xs"
						/>
					</div>

					<div class="flex flex-col gap-1.5">
						<label for="acc-token" class="text-xs font-medium text-foreground">
							Token or 9router Connection JSON
						</label>
						<Input
							id="acc-token"
							type="text"
							bind:value={tokenOrJson}
							placeholder="Paste refresh token (1//...) or JSON object"
							class="font-mono text-xs"
							required
						/>
						<span class="text-[11px] text-muted-foreground">
							Accepts Google refresh token, access token, or connection JSON export from 9router.
						</span>
					</div>
				</div>
			{/if}

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
						Connecting...
					{:else}
						Connect Account
					{/if}
				</Button>
			</Dialog.Footer>
		</form>
	</Dialog.Content>
</Dialog.Root>
