<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import * as Card from '$lib/components/ui/card';
	import { Button } from '$lib/components/ui/button';
	import { Badge } from '$lib/components/ui/badge';
	import { Switch } from '$lib/components/ui/switch';
	import { zaiState } from '$lib/state/zai.svelte';
	import { toast } from '$lib/state/toast.svelte';
	import { cn } from '$lib/utils';
	import ModelTestChat from './model-test-chat.svelte';
	import type { ProviderModel } from '$lib/types';

	import SparklesIcon from '@lucide/svelte/icons/sparkles';
	import DownloadIcon from '@lucide/svelte/icons/download';
	import ShieldIcon from '@lucide/svelte/icons/shield';
	import ShieldCheckIcon from '@lucide/svelte/icons/shield-check';
	import ActivityIcon from '@lucide/svelte/icons/activity';
	import WrenchIcon from '@lucide/svelte/icons/wrench';

	let selectedModel = $state('zai/x-preview-l');

	const rawModels: {
		id: string;
		name: string;
		desc: string;
		ctx: number;
		out: number;
		vision?: boolean;
		guest?: boolean;
	}[] = [
		{ id: 'zai/x-preview-l', name: 'GLM-5.3-Flash', desc: 'Lightweight flagship — fastest, vision capable', ctx: 200000, out: 128000, vision: true, guest: true },
		{ id: 'zai/glm-4.7', name: 'GLM-4.7', desc: 'Stable general model with agent mode', ctx: 200000, out: 40000, guest: true },
		{ id: 'zai/glm-5.3', name: 'GLM-5.3', desc: 'Current flagship — needs a signed-in account', ctx: 200000, out: 128000 },
		{ id: 'zai/glm-5.2', name: 'GLM-5.2', desc: 'Previous flagship — needs a signed-in account', ctx: 200000, out: 64064 },
		{ id: 'zai/GLM-5-Turbo', name: 'GLM-5-Turbo', desc: 'Speed-optimised — needs a signed-in account', ctx: 200000, out: 32000 },
		{ id: 'zai/GLM-5v-Turbo', name: 'GLM-5V-Turbo', desc: 'Vision capable — needs a signed-in account', ctx: 200000, out: 32000, vision: true },
		{ id: 'zai/GLM-4.1V-Thinking-FlashX', name: 'GLM-4.1V-9B-Thinking', desc: 'Compact vision reasoner — needs a signed-in account', ctx: 128000, out: 16000, vision: true },
		{ id: 'zai/0727-106B-API', name: 'GLM-4.5-Air', desc: 'Lightweight general — needs a signed-in account', ctx: 128000, out: 80000 },
		{ id: 'zai/0727-360B-API', name: 'GLM-4.5', desc: 'Large general — needs a signed-in account', ctx: 128000, out: 80000 },
		{ id: 'zai/0808-360B-DR', name: 'GLM-4.6-DR', desc: 'Deep-research tuned — needs a signed-in account', ctx: 128000, out: 32000 }
	];

	let providerModels = $derived<ProviderModel[]>(
		rawModels.map((m) => ({
			id: m.id,
			name: m.name,
			description: m.desc,
			object: 'model',
			owned_by: 'zai',
			enabled: zaiState.enabled && (m.guest ?? false),
			context_length: m.ctx,
			max_completion_tokens: m.out,
			capabilities: {
				tools: true,
				reasoning: true,
				thinking_can_disable: true,
				vision: m.vision ?? false,
				context_window: m.ctx,
				max_output: m.out
			}
		}))
	);

	onMount(() => {
		zaiState.load().then(() => {
			if (zaiState.enabled) zaiState.startStatusPolling();
		});
		zaiState.startRamPolling();
	});

	onDestroy(() => {
		zaiState.destroy();
	});

	async function handleToggleMain(checked: boolean) {
		if (checked && !zaiState.installed) {
			toast.error('Browser Required', 'Install the browser first — it mints the captcha token.');
			return;
		}
		const ok = await zaiState.toggle(checked);
		if (ok) {
			toast.success(
				checked ? 'z.ai Enabled' : 'z.ai Disabled',
				checked ? 'GLM models are now available via API' : 'Browser released'
			);
		} else {
			toast.error('Failed to update status', zaiState.error || 'Unknown error');
		}
	}

	async function handleInstall() {
		const ok = await zaiState.installBrowser();
		if (ok) toast.info('Download Started', 'Fetching the Chromium build used for captcha minting.');
		else toast.error('Download Failed', zaiState.error || 'Unknown error');
	}

	async function handlePassthrough(checked: boolean) {
		const ok = await zaiState.setPassthroughTools(checked);
		if (ok) {
			toast.success('Tool Routing Updated', checked ? 'Tool calls are handed to your agent.' : 'Tool calls stay inside the router.');
		} else {
			toast.error('Failed to update tool routing', zaiState.error || 'Unknown error');
		}
	}
</script>

<div class="space-y-6">
	<!-- Header -->
	<div class="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
		<div>
			<div class="flex items-center gap-2.5">
				<div class="flex size-9 items-center justify-center rounded-lg border border-primary/20 bg-primary/10 text-primary">
					<SparklesIcon class="size-5" />
				</div>
				<div>
					<h1 class="text-xl font-bold tracking-tight">z.ai (GLM)</h1>
					<p class="text-xs text-muted-foreground">
						Direct HTTPS completion with headless browser captcha minting
					</p>
				</div>
			</div>
		</div>

		<div class="flex items-center gap-3">
			<Badge variant={zaiState.enabled ? 'default' : 'secondary'} class="gap-1.5 px-2.5 py-1 text-xs">
				<span class={cn('size-1.5 rounded-full', zaiState.enabled ? 'bg-primary-foreground' : 'bg-muted-foreground')}></span>
				{zaiState.enabled ? 'Enabled' : 'Disabled'}
			</Badge>

			<div class="flex items-center gap-2 rounded-lg border border-border bg-card px-3 py-1.5">
				<span class="text-xs font-medium">Provider:</span>
				<Switch
					checked={zaiState.enabled}
					disabled={zaiState.toggling || zaiState.installing}
					onCheckedChange={handleToggleMain}
				/>
			</div>
		</div>
	</div>

	<!-- Status Grid: 3 columns like ChatGPT -->
	<div class="grid grid-cols-1 gap-4 md:grid-cols-3">
		<!-- Browser Status Card -->
		<Card.Root>
			<Card.Header class="pb-2">
				<Card.Title class="flex items-center gap-2 text-sm font-medium text-muted-foreground">
					<ShieldIcon class="size-4" />
					Stealth Browser
				</Card.Title>
			</Card.Header>
			<Card.Content>
				<div class="flex items-baseline justify-between">
					<span class="text-xl font-bold">
						{zaiState.installed ? 'Installed' : 'Not Installed'}
					</span>
					<Badge variant={zaiState.installed ? 'outline' : 'destructive'} class="text-xs">
						{zaiState.installed ? 'Ready' : 'Download Needed'}
					</Badge>
				</div>
				<p class="mt-1 truncate text-xs text-muted-foreground">
					{zaiState.browserPath || 'No browser binary detected'}
				</p>

				{#if !zaiState.installed && !zaiState.installing}
					<Button
						variant="outline"
						size="sm"
						class="mt-3 w-full gap-1.5 text-xs"
						onclick={handleInstall}
					>
						<DownloadIcon class="size-3.5" />
						Download Stealth Browser (~150 MB)
					</Button>
				{/if}

				{#if zaiState.installState === 'downloading' || zaiState.installState === 'extracting'}
					<div class="mt-3 space-y-1">
						<div class="flex justify-between text-xs text-muted-foreground">
							<span>{zaiState.installState === 'downloading' ? 'Downloading' : 'Extracting'}</span>
							<span>{zaiState.installProgress}%</span>
						</div>
						<div class="h-1.5 w-full overflow-hidden rounded-full bg-muted">
							<div class="h-full bg-primary transition-all" style="width: {zaiState.installProgress}%"></div>
						</div>
					</div>
				{/if}
			</Card.Content>
		</Card.Root>

		<!-- Session / Auth Card -->
		<Card.Root>
			<Card.Header class="pb-2 flex flex-row items-center justify-between space-y-0">
				<Card.Title class="flex items-center gap-2 text-sm font-medium text-muted-foreground">
					<ShieldCheckIcon class="size-4" />
					Session Mode
				</Card.Title>
			</Card.Header>
			<Card.Content>
				<div class="flex items-baseline justify-between">
					<span class="text-xl font-bold">
						{zaiState.guestMode ? 'Anonymous' : 'Authenticated'}
					</span>
					<Badge variant="outline" class="text-xs">
						{zaiState.guestMode ? 'Guest Token' : 'Account'}
					</Badge>
				</div>
				<p class="mt-1 text-xs text-muted-foreground">
					Auto-refreshed guest JWT. 2 of 10 models unlocked.
				</p>
			</Card.Content>
		</Card.Root>

		<!-- Live Browser RAM Usage Card -->
		<Card.Root>
			<Card.Header class="pb-2 flex flex-row items-center justify-between space-y-0">
				<Card.Title class="flex items-center gap-2 text-sm font-medium text-muted-foreground">
					<ActivityIcon class="size-4" />
					Live Browser RAM
				</Card.Title>
				<Badge variant="outline" class="gap-1 px-1.5 py-0 text-[10px] text-muted-foreground font-mono">
					<span class="size-1.5 rounded-full bg-emerald-500 animate-pulse"></span>
					2s Live
				</Badge>
			</Card.Header>
			<Card.Content>
				<div class="flex items-baseline justify-between">
					<div class="text-xl font-bold">
						{#if zaiState.browserRunning && zaiState.browserRssMb > 0}
							{zaiState.browserRssMb.toFixed(1)} MB
						{:else if zaiState.enabled}
							0.0 MB <span class="text-xs font-normal text-muted-foreground">(Idle)</span>
						{:else}
							0.0 MB <span class="text-xs font-normal text-muted-foreground">(Stopped)</span>
						{/if}
					</div>
					<Badge variant={zaiState.browserRunning ? 'default' : 'outline'} class="text-xs font-mono">
						{#if zaiState.browserRunning}
							{zaiState.browserProcs} procs (PID {zaiState.browserPid})
						{:else}
							0 procs
						{/if}
					</Badge>
				</div>
				<p class="mt-1 text-xs text-muted-foreground">
					Summed resident memory (RSS). Browser launches on demand to mint captcha.
				</p>
			</Card.Content>
		</Card.Root>
	</div>

	<!-- Tool Access Card -->
	<Card.Root>
		<Card.Header class="pb-2">
			<Card.Title class="flex items-center gap-2 text-sm font-medium text-muted-foreground">
				<WrenchIcon class="size-4" />
				Tool Routing
			</Card.Title>
			<Card.Description class="pt-1 text-xs">
				z.ai does not take a tools array over HTTP. The router emulates tools via prompt protocol and converts responses to OpenAI tool_calls.
			</Card.Description>
		</Card.Header>
		<Card.Content class="space-y-3">
			<div class="flex items-center justify-between rounded-lg border border-primary/30 bg-primary/5 p-3">
				<div class="min-w-0 pe-3">
					<div class="text-sm font-medium">Hand tool calls to the agent</div>
					<div class="text-xs text-muted-foreground">
						Return tool calls to your agent client. Off executes tools router-side.
					</div>
				</div>
				<Switch
					checked={zaiState.passthroughTools}
					onCheckedChange={handlePassthrough}
				/>
			</div>
		</Card.Content>
	</Card.Root>

	<ModelTestChat models={providerModels} bind:selectedModelId={selectedModel} />
</div>
