<script lang="ts">
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { cn } from '$lib/utils';
	import KeyRoundIcon from '@lucide/svelte/icons/key-round';
	import SunIcon from '@lucide/svelte/icons/sun';
	import MoonIcon from '@lucide/svelte/icons/moon';

	interface Props {
		error: string | null;
	}

	let { error }: Props = $props();
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
	class="sticky top-0 z-30 border-b border-border bg-card/75 px-4 py-3 backdrop-blur-md sm:px-8"
>
	<div class="mx-auto flex max-w-6xl items-center justify-between">
		<div class="flex items-center gap-3">
			<div
				class="flex size-9 items-center justify-center rounded-lg border border-primary/20 bg-primary/10 text-primary"
			>
				<KeyRoundIcon class="size-4" />
			</div>
			<div class="flex flex-col">
				<div class="flex items-center gap-2">
					<span class="text-base font-semibold tracking-tight">Lun Router</span>
					<Badge variant="secondary" class="text-[10px] font-medium">9router Go</Badge>
				</div>
				<span class="text-xs text-muted-foreground">API Key & OpenCode Free Completions</span>
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
					{error ? 'offline' : 'Go :8080'}
				</span>
			</div>

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
