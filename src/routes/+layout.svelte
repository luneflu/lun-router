<script lang="ts">
	import './layout.css';
	import favicon from '#lib/assets/favicon.svg';
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { keysState } from '$lib/state/keys.svelte';
	import { providerState } from '$lib/state/provider.svelte';
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

	const activeView = $derived<'home' | 'opencode'>(
		page.url.pathname.startsWith('/opencode') ? 'opencode' : 'home'
	);

	onMount(() => {
		keysState.load();
		providerState.load();
	});
</script>

<svelte:head>
	<link rel="icon" href={favicon} />
</svelte:head>

<div class="flex min-h-screen bg-background text-foreground">
	<!-- Left Navigation Sidebar -->
	<Sidebar
		{activeView}
		totalKeys={keysState.totalCount}
		enabledModels={providerState.enabledCount}
		totalModels={providerState.totalCount}
		mobileOpen={mobileMenuOpen}
		onCloseMobile={() => (mobileMenuOpen = false)}
	/>

	<!-- Main Content Area -->
	<div class="flex flex-1 flex-col overflow-x-hidden">
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
						<span>{keysState.error}. Please run <code>go run .</code> from the project root.</span>
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

<Toaster />
