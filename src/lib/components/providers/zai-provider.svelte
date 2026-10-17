<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import * as Card from '$lib/components/ui/card';
	import { Button } from '$lib/components/ui/button';
	import { Badge } from '$lib/components/ui/badge';
	import { Switch } from '$lib/components/ui/switch';
	import { zaiState } from '$lib/state/zai.svelte';
	import { toast } from '$lib/state/toast.svelte';
	import ModelTestChat from './model-test-chat.svelte';
	import type { ProviderModel } from '$lib/types';

	import SparklesIcon from '@lucide/svelte/icons/sparkles';
	import DownloadIcon from '@lucide/svelte/icons/download';
	import ShieldCheckIcon from '@lucide/svelte/icons/shield-check';
	import WrenchIcon from '@lucide/svelte/icons/wrench';
	import GaugeIcon from '@lucide/svelte/icons/gauge';

	let selectedModel = $state('zai/x-preview-l');

	// Mirrors internal/zai/models.go Catalog: only models that can drive a tool
	// loop are advertised, because the rest cannot support an agent client.
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
		// These exist in the catalog and are agent-capable, but an anonymous
		// session gets 403 "Model not available for current user level" for
		// every one of them, so they are listed as unavailable rather than
		// offered as working choices.
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

<div class="space-y-4">
	<Card.Root>
		<Card.Header class="pb-2">
			<Card.Title class="flex items-center gap-2 text-sm font-medium text-muted-foreground">
				<SparklesIcon class="h-4 w-4" />
				z.ai (GLM)
			</Card.Title>
			<Card.Description class="pt-1 text-xs">
				Chat traffic goes straight over HTTPS. A browser is used for one thing only — minting the
				Aliyun captcha token, which is single-use and cannot be computed without running Aliyun's
				device-signing SDK.
			</Card.Description>
		</Card.Header>
		<Card.Content class="space-y-4">
			<div class="flex items-center justify-between">
				<div class="space-y-0.5">
					<div class="text-sm font-medium">Enable provider</div>
					<div class="text-xs text-muted-foreground">
						Exposes {rawModels.length} GLM models under <code>zai/</code>.
					</div>
				</div>
				<Switch
					checked={zaiState.enabled}
					disabled={zaiState.toggling || zaiState.loading}
					onCheckedChange={handleToggleMain}
				/>
			</div>

			<div class="flex flex-wrap items-center gap-2">
				<Badge variant={zaiState.enabled ? 'default' : 'secondary'}>
					{zaiState.enabled ? 'Enabled' : 'Disabled'}
				</Badge>
				<Badge variant={zaiState.installed ? 'default' : 'secondary'}>
					{zaiState.installed ? 'Browser installed' : 'Browser missing'}
				</Badge>
				<Badge variant={zaiState.browserRunning ? 'default' : 'secondary'}>
					{zaiState.browserRunning ? 'Browser running' : 'Browser idle'}
				</Badge>
				<Badge variant="outline">
					<ShieldCheckIcon class="mr-1 h-3 w-3" />
					Guest session
				</Badge>
			</div>

			{@const state = zaiState.installState}
			{#if state === 'downloading' || state === 'extracting'}
				<div class="space-y-1">
					<div class="flex justify-between text-xs text-muted-foreground">
						<span>{state === 'downloading' ? 'Downloading browser' : 'Extracting'}</span>
						<span>{zaiState.installProgress}%</span>
					</div>
					<div class="h-1.5 w-full overflow-hidden rounded-full bg-muted">
						<div class="h-full bg-primary transition-all" style="width: {zaiState.installProgress}%"></div>
					</div>
				</div>
			{:else if !zaiState.installed}
				<Button size="sm" onclick={handleInstall} disabled={zaiState.installing}>
					<DownloadIcon class="mr-2 h-4 w-4" />
					{zaiState.installing ? 'Starting…' : 'Install browser'}
				</Button>
			{/if}

			{#if zaiState.installError}
				<p class="text-xs text-destructive">{zaiState.installError}</p>
			{/if}
			{#if zaiState.error}
				<p class="text-xs text-destructive">{zaiState.error}</p>
			{/if}

			{#if zaiState.browserPath}
				<p class="text-[11px] break-all text-muted-foreground">
					Executable: <code>{zaiState.browserPath}</code>
				</p>
			{/if}
		</Card.Content>
	</Card.Root>

	<Card.Root>
		<Card.Header class="pb-2">
			<Card.Title class="flex items-center gap-2 text-sm font-medium text-muted-foreground">
				<WrenchIcon class="h-4 w-4" />
				Tool Routing
			</Card.Title>
		</Card.Header>
		<Card.Content class="space-y-3">
			<div class="flex items-center justify-between">
				<div class="space-y-0.5">
					<div class="text-sm font-medium">Hand tool calls to the agent</div>
					<div class="text-xs text-muted-foreground">
						Returns the model's tool call to your client so it runs in its own sandbox. Off means the
						router executes tools itself.
					</div>
				</div>
				<Switch
					checked={zaiState.passthroughTools}
					onCheckedChange={handlePassthrough}
				/>
			</div>
			<p class="text-[11px] text-muted-foreground">
				z.ai takes no <code>tools</code> field — sending one fails upstream. Tool use runs through a
				prompt protocol that GLM follows reliably; the router parses the call and returns standard
				OpenAI <code>tool_calls</code>.
			</p>
		</Card.Content>
	</Card.Root>

	<Card.Root>
		<Card.Header class="pb-2">
			<Card.Title class="flex items-center gap-2 text-sm font-medium text-muted-foreground">
				<GaugeIcon class="h-4 w-4" />
				Model Notes
			</Card.Title>
		</Card.Header>
		<Card.Content class="space-y-2 text-xs text-muted-foreground">
			<p>
				Verified against a live anonymous token: only <code>zai/x-preview-l</code> and
				<code>zai/glm-4.7</code> answer. The other eight return
				<code>403 Model not available for current user level</code>, so the router does not advertise
				them — supplying a signed-in token would unlock the rest.
			</p>
			<p>
				Tool calls are verified end to end: a read/call round trip returns a real answer built from the
				file contents rather than a guess.
			</p>
			<p>
				Reasoning is a separate stream phase and is returned as <code>reasoning_content</code>. The
				router maps <code>reasoning_effort</code> onto low / medium / max.
			</p>
		</Card.Content>
	</Card.Root>

	<ModelTestChat models={providerModels} bind:selectedModelId={selectedModel} />
</div>
