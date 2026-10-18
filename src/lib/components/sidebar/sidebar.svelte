<script lang="ts">
	import { Badge } from '$lib/components/ui/badge';
	import { cn } from '$lib/utils';
	import LayoutDashboardIcon from '@lucide/svelte/icons/layout-dashboard';
	import ActivityIcon from '@lucide/svelte/icons/activity';
	import BotIcon from '@lucide/svelte/icons/bot';
	import RocketIcon from '@lucide/svelte/icons/rocket';
	import KeyRoundIcon from '@lucide/svelte/icons/key-round';
	import ZapIcon from '@lucide/svelte/icons/zap';
	import XIcon from '@lucide/svelte/icons/x';
	import CodeIcon from '@lucide/svelte/icons/code';
	import { Button } from '$lib/components/ui/button';

	interface Props {
		activeView: 'home' | 'usage' | 'opencode' | 'antigravity' | 'kilocode' | 'chatgpt' | 'zai' | 'token-saver';
		onSelectView?: (view: 'home' | 'usage' | 'opencode' | 'antigravity' | 'kilocode' | 'chatgpt' | 'zai' | 'token-saver') => void;
		totalKeys: number;
		enabledModels: number;
		totalModels: number;
		antigravityAccounts?: number;
		antigravityEnabledModels?: number;
		antigravityTotalModels?: number;
		kilocodeAccounts?: number;
		kilocodeEnabledModels?: number;
		kilocodeTotalModels?: number;
		mobileOpen: boolean;
		onCloseMobile: () => void;
	}

	let {
		activeView,
		onSelectView,
		totalKeys,
		enabledModels,
		totalModels,
		antigravityAccounts = 0,
		antigravityEnabledModels = 0,
		antigravityTotalModels = 0,
		kilocodeAccounts = 0,
		kilocodeEnabledModels = 0,
		kilocodeTotalModels = 0,
		mobileOpen,
		onCloseMobile
	}: Props = $props();

	function handleNavigate(view: 'home' | 'usage' | 'opencode' | 'antigravity' | 'kilocode' | 'chatgpt' | 'zai' | 'token-saver') {
		onSelectView?.(view);
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
		'fixed inset-y-0 start-0 z-40 flex h-screen w-64 flex-col border-e border-border bg-card transition-transform duration-200 md:translate-x-0',
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

			<a
				href="/"
				class={cn(
					'flex items-center justify-between rounded-lg px-2.5 py-2 text-xs font-medium transition-colors',
					activeView === 'home'
						? 'bg-primary/10 font-semibold text-primary'
						: 'text-muted-foreground hover:bg-muted hover:text-foreground'
				)}
				onclick={() => handleNavigate('home')}
			>
				<div class="flex items-center gap-2.5">
					<LayoutDashboardIcon class="size-4" />
					<span>Home</span>
				</div>
				<Badge variant="outline" class="font-mono text-[10px]">
					{totalKeys}
				</Badge>
			</a>

			<a
				href="/usage"
				class={cn(
					'flex items-center justify-between rounded-lg px-2.5 py-2 text-xs font-medium transition-colors',
					activeView === 'usage'
						? 'bg-primary/10 font-semibold text-primary'
						: 'text-muted-foreground hover:bg-muted hover:text-foreground'
				)}
				onclick={() => handleNavigate('usage')}
			>
				<div class="flex items-center gap-2.5">
					<ActivityIcon class="size-4" />
					<span>Usage</span>
				</div>
			</a>
		</div>

		<!-- Providers Section -->
		<div class="flex flex-col gap-1">
			<div class="flex items-center justify-between px-2.5">
				<span class="text-[10px] font-semibold tracking-wider text-muted-foreground uppercase">
					Providers
				</span>
				<span class="font-mono text-[10px] text-muted-foreground">
					{1 + (antigravityAccounts > 0 ? 1 : 0) + (kilocodeAccounts > 0 ? 1 : 0)} active
				</span>
			</div>

			<a
				href="/opencode"
				class={cn(
					'flex items-center justify-between rounded-lg px-2.5 py-2 text-xs font-medium transition-colors',
					activeView === 'opencode'
						? 'bg-primary/10 font-semibold text-primary'
						: 'text-muted-foreground hover:bg-muted hover:text-foreground'
				)}
				onclick={() => handleNavigate('opencode')}
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
			</a>

			<a
				href="/antigravity"
				class={cn(
					'flex items-center justify-between rounded-lg px-2.5 py-2 text-xs font-medium transition-colors',
					activeView === 'antigravity'
						? 'bg-amber-500/10 font-semibold text-amber-500'
						: 'text-muted-foreground hover:bg-muted hover:text-foreground'
				)}
				onclick={() => handleNavigate('antigravity')}
			>
				<div class="flex items-center gap-2.5">
					<div class="relative flex items-center justify-center">
						<RocketIcon class="size-4" />
						<span
							class={cn(
								'absolute -end-0.5 -top-0.5 size-1.5 rounded-full',
								antigravityAccounts > 0 ? 'bg-amber-500' : 'bg-muted-foreground/40'
							)}
							title={antigravityAccounts > 0 ? 'Active accounts' : 'No accounts connected'}
						></span>
					</div>
					<div class="flex flex-col items-start text-start">
						<span>Antigravity</span>
					</div>
				</div>

				<div class="flex items-center gap-1.5">
					{#if antigravityTotalModels > 0}
						<span class="font-mono text-[10px] text-muted-foreground">
							{antigravityEnabledModels}/{antigravityTotalModels}
						</span>
					{/if}
				</div>
			</a>

			<a
				href="/kilocode"
				class={cn(
					'flex items-center justify-between rounded-lg px-2.5 py-2 text-xs font-medium transition-colors',
					activeView === 'kilocode'
						? 'bg-orange-500/10 font-semibold text-orange-500'
						: 'text-muted-foreground hover:bg-muted hover:text-foreground'
				)}
				onclick={() => handleNavigate('kilocode')}
			>
				<div class="flex items-center gap-2.5">
					<div class="relative flex items-center justify-center">
						<CodeIcon class="size-4" />
						<span
							class={cn(
								'absolute -end-0.5 -top-0.5 size-1.5 rounded-full',
								kilocodeAccounts > 0 ? 'bg-orange-500' : 'bg-muted-foreground/40'
							)}
							title={kilocodeAccounts > 0 ? 'Active accounts' : 'No accounts connected'}
						></span>
					</div>
					<div class="flex flex-col items-start text-start">
						<span>Kilo Code</span>
					</div>
				</div>

				<div class="flex items-center gap-1.5">
					{#if kilocodeTotalModels > 0}
						<span class="font-mono text-[10px] text-muted-foreground">
							{kilocodeEnabledModels}/{kilocodeTotalModels}
						</span>
					{/if}
				</div>
			</a>
		</div>

		<!-- Token Saver Section -->
		<div class="flex flex-col gap-1">
			<span class="px-2.5 text-[10px] font-semibold tracking-wider text-muted-foreground uppercase">
				Optimization
			</span>

			<a
				href="/token-saver"
				class={cn(
					'flex items-center justify-between rounded-lg px-2.5 py-2 text-xs font-medium transition-colors',
					activeView === 'token-saver'
						? 'bg-amber-500/10 font-semibold text-amber-500'
						: 'text-muted-foreground hover:bg-muted hover:text-foreground'
				)}
				onclick={() => handleNavigate('token-saver')}
			>
				<div class="flex items-center gap-2.5">
					<div class="relative flex items-center justify-center">
						<ZapIcon class="size-4" />
					</div>
					<div class="flex flex-col items-start text-start">
						<span>Token Saver</span>
					</div>
				</div>
			</a>
		</div>
	</div>
</aside>
