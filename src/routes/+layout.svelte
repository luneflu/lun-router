<script lang="ts">
	import './layout.css';
	import favicon from '#lib/assets/favicon.svg';
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { authState } from '$lib/state/auth.svelte';
	import AuthGate from '$lib/components/auth/auth-gate.svelte';
	import { keysState } from '$lib/state/keys.svelte';
	import { providerState } from '$lib/state/provider.svelte';
	import { antigravityState } from '$lib/state/antigravity.svelte';
	import { compactorState } from '$lib/state/compactor.svelte';
	import { ponytailState } from '$lib/state/ponytail.svelte';
	import { kilocodeState } from '$lib/state/kilocode.svelte';
	import Sidebar from '$lib/components/sidebar/sidebar.svelte';
	import Navbar from '$lib/components/navbar/navbar.svelte';
	import * as Alert from '$lib/components/ui/alert';
	import { Button } from '$lib/components/ui/button';
	import AlertCircleIcon from '@lucide/svelte/icons/alert-circle';
	import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';
	import Toaster from '$lib/components/ui/toast/toaster.svelte';
	import type { LayoutProps } from './$types';

	let { children }: LayoutProps = $props();

	let mobileMenuOpen = $state(false);

	const activeView = $derived<'home' | 'usage' | 'opencode' | 'antigravity' | 'kilocode' | 'chatgpt' | 'zai' | 'token-saver'>(
		page.url.pathname.startsWith('/chatgpt')
			? 'chatgpt'
			: page.url.pathname.startsWith('/zai')
				? 'zai'
				: page.url.pathname.startsWith('/antigravity')
				? 'antigravity'
				: page.url.pathname.startsWith('/kilocode')
					? 'kilocode'
					: page.url.pathname.startsWith('/opencode')
					? 'opencode'
					: page.url.pathname.startsWith('/token-saver')
						? 'token-saver'
						: page.url.pathname.startsWith('/usage')
							? 'usage'
							: 'home'
	);

	function loadData() {
		keysState.load();
		providerState.load();
		antigravityState.load();
		kilocodeState.load();
		compactorState.load();
		ponytailState.load();
	}

	onMount(async () => {
		await authState.checkStatus();
		if (!authState.authEnabled || authState.authenticated) {
			loadData();
		}
	});
</script>

<svelte:head>
	<link rel="icon" href={favicon} />
</svelte:head>

{#if authState.loading}
	<div class="flex min-h-screen items-center justify-center bg-background">
		<div
			class="size-6 animate-spin rounded-full border-2 border-primary border-t-transparent"
		></div>
	</div>
{:else if authState.setupRequired || (authState.authEnabled && !authState.authenticated)}
	<AuthGate onSuccess={loadData} />
{:else}
	<div class="min-h-screen bg-background text-foreground">
		<!-- Left Navigation Sidebar -->
		<Sidebar
			{activeView}
			totalKeys={keysState.totalCount}
			enabledModels={providerState.enabledCount}
			totalModels={providerState.totalCount}
			antigravityAccounts={antigravityState.activeAccountsCount}
			antigravityEnabledModels={antigravityState.enabledCount}
			antigravityTotalModels={antigravityState.totalCount}
			kilocodeAccounts={kilocodeState.activeAccountsCount}
			kilocodeEnabledModels={kilocodeState.enabledCount}
			kilocodeTotalModels={kilocodeState.totalCount}
			mobileOpen={mobileMenuOpen}
			onCloseMobile={() => (mobileMenuOpen = false)}
		/>

		<!-- Main Content Area -->
		<div class="flex min-h-screen min-w-0 flex-1 flex-col md:ps-64">
			<Navbar
				error={keysState.error || providerState.error}
				{activeView}
				onToggleMobileMenu={() => (mobileMenuOpen = !mobileMenuOpen)}
			/>

			<main class="mx-auto flex w-full max-w-6xl flex-1 flex-col gap-6 px-4 py-6 sm:px-8">
				{#if keysState.error}
					<Alert.Root variant="destructive">
						<AlertCircleIcon class="size-4" />
						<Alert.Title>Go Backend Unreachable</Alert.Title>
						<Alert.Description class="flex items-center justify-between gap-4">
							<span>{keysState.error}. Please run <code>go run .</code> from the project root.</span
							>
							<Button
								variant="outline"
								size="sm"
								onclick={() => {
									keysState.load();
									providerState.load();
								}}
							>
								<RefreshCwIcon data-icon="inline-start" />
								Retry
							</Button>
						</Alert.Description>
					</Alert.Root>
				{/if}

				{@render children()}
			</main>
		</div>
	</div>
{/if}

<Toaster />
