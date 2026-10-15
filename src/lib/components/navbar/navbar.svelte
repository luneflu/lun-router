<script lang="ts">
	import { Button } from '$lib/components/ui/button';
	import { cn } from '$lib/utils';
	import { authState } from '$lib/state/auth.svelte';
	import SunIcon from '@lucide/svelte/icons/sun';
	import MoonIcon from '@lucide/svelte/icons/moon';
	import MenuIcon from '@lucide/svelte/icons/menu';
	import ChevronRightIcon from '@lucide/svelte/icons/chevron-right';
	import LockIcon from '@lucide/svelte/icons/lock';

	interface Props {
		error: string | null;
		activeView: 'home' | 'opencode' | 'antigravity' | 'token-saver';
		onToggleMobileMenu?: () => void;
	}

	let { error, activeView, onToggleMobileMenu }: Props = $props();
	let isDark = $state(true);

	function toggleTheme() {
		isDark = !isDark;
		const root = document.documentElement;
		if (isDark) {
			root.classList.add('dark');
		} else {
			root.classList.remove('dark');
		}
	}
</script>

<header
	class="sticky top-0 z-30 flex h-16 w-full shrink-0 items-center border-b border-border bg-card/85 px-4 backdrop-blur-md sm:px-8"
>
	<div class="flex w-full items-center justify-between">
		<div class="flex items-center gap-3">
			<Button
				variant="ghost"
				size="icon-sm"
				class="md:hidden"
				onclick={onToggleMobileMenu}
				aria-label="Toggle navigation menu"
			>
				<MenuIcon class="size-4" />
			</Button>

			<!-- Breadcrumb -->
			<div class="flex items-center gap-1.5 text-xs">
				<span class="font-medium text-muted-foreground">Dashboard</span>
				<ChevronRightIcon class="size-3 text-muted-foreground/60" />
				{#if activeView === 'home'}
					<span class="font-semibold text-foreground">Home</span>
				{:else if activeView === 'antigravity'}
					<span class="text-muted-foreground">Providers</span>
					<ChevronRightIcon class="size-3 text-muted-foreground/60" />
					<span class="font-semibold text-foreground">Google Antigravity</span>
				{:else if activeView === 'token-saver'}
					<span class="text-muted-foreground">Optimization</span>
					<ChevronRightIcon class="size-3 text-muted-foreground/60" />
					<span class="font-semibold text-foreground">Token Saver</span>
				{:else}
					<span class="text-muted-foreground">Providers</span>
					<ChevronRightIcon class="size-3 text-muted-foreground/60" />
					<span class="font-semibold text-foreground">OpenCode Free</span>
				{/if}
			</div>
		</div>

		<div class="flex items-center gap-2 sm:gap-3">
			<div class="hidden items-center gap-2 rounded-full bg-muted px-2.5 py-1 text-xs sm:flex">
				<span
					class={cn(
						'size-2 rounded-full',
						error ? 'animate-pulse bg-destructive' : 'bg-emerald-500'
					)}
				></span>
				<span class="text-muted-foreground">Backend:</span>
				<span
					class={cn(
						'font-mono text-[11px]',
						error ? 'font-medium text-destructive' : 'text-foreground'
					)}
				>
					{error ? 'offline' : 'Go :20128'}
				</span>
			</div>

			{#if authState.authEnabled && authState.authenticated}
				<Button
					variant="ghost"
					size="icon-sm"
					onclick={() => authState.logout()}
					aria-label="Lock dashboard"
					title="Lock dashboard (logout)"
				>
					<LockIcon class="size-4 text-muted-foreground hover:text-foreground" />
				</Button>
			{/if}

			<Button
				variant="ghost"
				size="icon-sm"
				onclick={toggleTheme}
				aria-label={isDark ? 'Switch to light theme' : 'Switch to dark theme'}
				title={isDark ? 'Switch to light theme' : 'Switch to dark theme'}
			>
				{#if isDark}
					<SunIcon class="size-4" />
				{:else}
					<MoonIcon class="size-4" />
				{/if}
			</Button>
		</div>
	</div>
</header>
