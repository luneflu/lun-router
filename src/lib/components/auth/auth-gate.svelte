<script lang="ts">
	import { authState } from '$lib/state/auth.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import * as Card from '$lib/components/ui/card';
	import * as Alert from '$lib/components/ui/alert';
	import LockIcon from '@lucide/svelte/icons/lock';
	import KeyRoundIcon from '@lucide/svelte/icons/key-round';
	import EyeIcon from '@lucide/svelte/icons/eye';
	import EyeOffIcon from '@lucide/svelte/icons/eye-off';
	import ShieldCheckIcon from '@lucide/svelte/icons/shield-check';
	import AlertCircleIcon from '@lucide/svelte/icons/alert-circle';
	import Loader2Icon from '@lucide/svelte/icons/loader-2';

	interface Props {
		onSuccess?: () => void;
	}

	let { onSuccess }: Props = $props();

	let password = $state('');
	let showPassword = $state(false);

	async function handleSubmit(e: SubmitEvent) {
		e.preventDefault();
		if (!password.trim() || authState.submitting) return;

		const ok = authState.setupRequired
			? await authState.setup(password.trim())
			: await authState.login(password.trim());

		if (ok) {
			password = '';
			onSuccess?.();
		}
	}
</script>

<div class="flex min-h-screen items-center justify-center bg-background px-4 py-12">
	<Card.Root class="w-full max-w-md border-border/80 bg-card shadow-lg">
		<Card.Header class="space-y-3 text-center">
			<div
				class="mx-auto flex size-12 items-center justify-center rounded-xl border border-primary/20 bg-primary/10 text-primary shadow-xs"
			>
				{#if authState.setupRequired}
					<ShieldCheckIcon class="size-6" />
				{:else}
					<LockIcon class="size-6" />
				{/if}
			</div>

			<div class="space-y-1">
				<Card.Title class="text-xl font-bold tracking-tight">
					{#if authState.setupRequired}
						Set Master Password
					{:else}
						Gateway Locked
					{/if}
				</Card.Title>
				<Card.Description class="text-xs text-muted-foreground">
					{#if authState.setupRequired}
						Create a password to secure your gateway. Saved as an irreversible bcrypt hash.
					{:else}
						Enter password to unlock your Lun Router dashboard.
					{/if}
				</Card.Description>
			</div>
		</Card.Header>

		<Card.Content>
			<form onsubmit={handleSubmit} class="space-y-4">
				{#if authState.error}
					<Alert.Root variant="destructive" class="py-2.5">
						<AlertCircleIcon class="size-4" />
						<Alert.Description class="text-xs">{authState.error}</Alert.Description>
					</Alert.Root>
				{/if}

				<div class="space-y-1.5">
					<div class="relative flex items-center">
						<Input
							type={showPassword ? 'text' : 'password'}
							bind:value={password}
							placeholder={authState.setupRequired
								? 'Enter new password (min 4 chars)'
								: 'Enter password'}
							autocomplete={authState.setupRequired ? 'new-password' : 'current-password'}
							autofocus
							required
							class="pe-10 font-mono text-sm"
							disabled={authState.submitting}
						/>
						<button
							type="button"
							class="absolute end-2 flex size-7 items-center justify-center text-muted-foreground transition-colors hover:text-foreground"
							onclick={() => (showPassword = !showPassword)}
							tabindex="-1"
							aria-label={showPassword ? 'Hide password' : 'Show password'}
						>
							{#if showPassword}
								<EyeOffIcon class="size-4" />
							{:else}
								<EyeIcon class="size-4" />
							{/if}
						</button>
					</div>
					{#if authState.setupRequired}
						<p class="text-[11px] text-muted-foreground">
							Password will be hashed using bcrypt (cost 10) and stored in <code
								>data/auth.json</code
							>.
						</p>
					{/if}
				</div>

				<Button
					type="submit"
					class="w-full gap-2 font-medium"
					disabled={authState.submitting || !password.trim()}
				>
					{#if authState.submitting}
						<Loader2Icon class="size-4 animate-spin" />
						<span>{authState.setupRequired ? 'Setting Password...' : 'Unlocking...'}</span>
					{:else if authState.setupRequired}
						<ShieldCheckIcon class="size-4" />
						<span>Set Password & Unlock</span>
					{:else}
						<KeyRoundIcon class="size-4" />
						<span>Unlock Dashboard</span>
					{/if}
				</Button>
			</form>
		</Card.Content>
	</Card.Root>
</div>
