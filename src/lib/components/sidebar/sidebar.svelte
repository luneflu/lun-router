<script lang="ts">
	import { Badge } from '$lib/components/ui/badge';
	import { cn } from '$lib/utils';
	import LayoutDashboardIcon from '@lucide/svelte/icons/layout-dashboard';
	import BotIcon from '@lucide/svelte/icons/bot';
	import KeyRoundIcon from '@lucide/svelte/icons/key-round';
	import CheckCircle2Icon from '@lucide/svelte/icons/check-circle-2';
	import XIcon from '@lucide/svelte/icons/x';
	import { Button } from '$lib/components/ui/button';

	interface Props {
		activeView: 'home' | 'opencode';
		onSelectView: (view: 'home' | 'opencode') => void;
		totalKeys: number;
		enabledModels: number;
		totalModels: number;
		mobileOpen: boolean;
		onCloseMobile: () => void;
	}

	let {
		activeView,
		onSelectView,
		totalKeys,
		enabledModels,
		totalModels,
		mobileOpen,
		onCloseMobile
	}: Props = $props();

	function select(view: 'home' | 'opencode') {
		onSelectView(view);
		onCloseMobile();
	}
</script>

<!-- Mobile backdrop -->
{#if mobileOpen}
	<div
		class="fixed inset-0 z-40 bg-background/80 backdrop-blur-xs md:hidden"
		onclick={onCloseMobile}
		aria-hidden="true"
	></div>
{/if}

<aside
	class={cn(
		'fixed inset-y-0 start-0 z-50 flex w-64 flex-col border-e border-border bg-card transition-transform duration-200 md:static md:translate-x-0',
		mobileOpen ? 'translate-x-0' : '-translate-x-full'
	)}
>
	<!-- Sidebar Header -->
	<div class="flex h-16 items-center justify-between border-b border-border px-5">
		<div class="flex items-center gap-2.5">
			<div
				class="flex size-8 items-center justify-center rounded-lg border border-primary/20 bg-primary/10 text-primary shadow-xs"
			>
				<KeyRoundIcon class="size-4" />
			</div>
			<div class="flex flex-col">
				<div class="flex items-center gap-1.5">
					<span class="text-sm font-bold tracking-tight">Lun Router</span>
					<Badge variant="secondary" class="h-4 px-1 text-[9px] font-medium">v1.0</Badge>
				</div>
				<span class="text-[11px] text-muted-foreground">AI Gateway & Proxy</span>
			</div>
		</div>

		<Button
			variant="ghost"
			size="icon-sm"
			class="md:hidden"
			onclick={onCloseMobile}
			aria-label="Close menu"
		>
			<XIcon class="size-4" />
		</Button>
	</div>

	<!-- Navigation Groups -->
	<div class="flex flex-1 flex-col gap-6 overflow-y-auto p-3">
		<!-- Main Navigation -->
		<div class="flex flex-col gap-1">
			<span class="px-2.5 text-[10px] font-semibold tracking-wider text-muted-foreground uppercase">
				General
			</span>

			<button
				type="button"
				class={cn(
					'flex items-center justify-between rounded-lg px-2.5 py-2 text-xs font-medium transition-colors',
					activeView === 'home'
						? 'bg-primary/10 font-semibold text-primary'
						: 'text-muted-foreground hover:bg-muted hover:text-foreground'
				)}
				onclick={() => select('home')}
			>
				<div class="flex items-center gap-2.5">
					<LayoutDashboardIcon class="size-4" />
					<span>Home</span>
				</div>
				<Badge variant="outline" class="font-mono text-[10px]">
					{totalKeys}
				</Badge>
			</button>
		</div>

		<!-- Providers Section -->
		<div class="flex flex-col gap-1">
			<div class="flex items-center justify-between px-2.5">
				<span class="text-[10px] font-semibold tracking-wider text-muted-foreground uppercase">
					Providers
				</span>
				<span class="font-mono text-[10px] text-muted-foreground">1 active</span>
			</div>

			<button
				type="button"
				class={cn(
					'flex items-center justify-between rounded-lg px-2.5 py-2 text-xs font-medium transition-colors',
					activeView === 'opencode'
						? 'bg-primary/10 font-semibold text-primary'
						: 'text-muted-foreground hover:bg-muted hover:text-foreground'
				)}
				onclick={() => select('opencode')}
			>
				<div class="flex items-center gap-2.5">
					<div class="relative flex items-center justify-center">
						<BotIcon class="size-4" />
						<span
							class="absolute -end-0.5 -top-0.5 size-1.5 rounded-full bg-emerald-500"
							title="Active provider"
						></span>
					</div>
					<div class="flex flex-col items-start text-start">
						<span>OpenCode Free</span>
					</div>
				</div>

				<div class="flex items-center gap-1.5">
					{#if totalModels > 0}
						<span class="font-mono text-[10px] text-muted-foreground">
							{enabledModels}/{totalModels}
						</span>
					{/if}
				</div>
			</button>
		</div>
	</div>
</aside>
