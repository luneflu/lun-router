<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import * as Card from '$lib/components/ui/card';
	import * as Dialog from '$lib/components/ui/dialog';
	import * as Table from '$lib/components/ui/table';
	import { Button } from '$lib/components/ui/button';
	import { Badge } from '$lib/components/ui/badge';
	import { Switch } from '$lib/components/ui/switch';
	import { cn } from '$lib/utils';
	import { chatgptState } from '$lib/state/chatgpt.svelte';
	import { toast } from '$lib/state/toast.svelte';
	import ModelTestChat from './model-test-chat.svelte';
	import type { ProviderModel } from '$lib/types';

	import BotIcon from '@lucide/svelte/icons/bot';
	import DownloadIcon from '@lucide/svelte/icons/download';
	import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';
	import ShieldIcon from '@lucide/svelte/icons/shield';
	import ActivityIcon from '@lucide/svelte/icons/activity';
	import KeyRoundIcon from '@lucide/svelte/icons/key-round';
	import UploadIcon from '@lucide/svelte/icons/upload';
	import Trash2Icon from '@lucide/svelte/icons/trash-2';
	import WrenchIcon from '@lucide/svelte/icons/wrench';

	let confirmInstallOpen = $state(false);
	let uploadCookiesOpen = $state(false);
	let cookieInputText = $state('');
	let fileInputRef = $state<HTMLInputElement | null>(null);
	let workDirInput = $state('');

	let selectedChatModel = $state('chatgpt/auto');

	const rawModels = [
		{ id: 'chatgpt/auto', name: 'Auto', desc: 'Dynamic model selection and fast routing' },
		{ id: 'chatgpt/gpt-6', name: 'GPT-6', desc: 'Latest reasoning model for complex problem solving' },
		{ id: 'chatgpt/gpt-6-mini', name: 'GPT-6 Mini', desc: 'Fast, lightweight reasoning model variant' },
		{ id: 'chatgpt/gpt-5-6', name: 'GPT-5.6 Luna', desc: 'Advanced flagship omni model' },
		{ id: 'chatgpt/gpt-5-6-mini', name: 'GPT-5.6 Luna Mini', desc: 'Speed-optimized Luna model' },
		{ id: 'chatgpt/gpt-5-3-mini', name: 'GPT-5.3 Mini', desc: 'Efficient lightweight model' }
	];

	let providerModels = $derived<ProviderModel[]>(
		rawModels.map((m) => ({
			id: m.id,
			name: m.name,
			description: m.desc,
			object: 'model',
			owned_by: 'chatgpt',
			enabled: chatgptState.enabled,
			capabilities: {
				tools: true,
				reasoning: m.id.includes('gpt-6'),
				thinking_can_disable: false
			}
		}))
	);

	onMount(() => {
		chatgptState.startRamPolling();
		workDirInput = chatgptState.workDir;
	});

	onDestroy(() => {
		chatgptState.stopRamPolling();
	});

	async function handleToggleMain(checked: boolean) {
		if (checked && !chatgptState.installed) {
			confirmInstallOpen = true;
			return;
		}

		const ok = await chatgptState.toggle(checked);
		if (ok) {
			toast.success(
				checked ? 'ChatGPT Web Enabled' : 'ChatGPT Web Disabled',
				checked ? 'Models are now available via API' : 'Browser process stopped'
			);
		} else {
			toast.error('Failed to update status', chatgptState.error || 'Unknown error');
		}
	}

	async function handleConfirmDownload() {
		try {
			await chatgptState.startInstall();
			toast.info('Download Started', 'Downloading stealth Chromium browser in background...');
		} catch (e: unknown) {
			toast.error('Download Failed', e instanceof Error ? e.message : 'Could not start download');
		}
	}

	async function handleUploadCookiesSubmit() {
		if (!cookieInputText.trim()) {
			toast.error('Error', 'Cookie JSON content is empty');
			return;
		}
		try {
			// Validate client-side first
			JSON.parse(cookieInputText.trim());
		} catch {
			toast.error('Invalid JSON', 'Input must be valid JSON array of cookies');
			return;
		}

		const ok = await chatgptState.uploadCookies(cookieInputText.trim());
		if (ok) {
			toast.success('Cookies Saved', 'ChatGPT authentication cookies stored');
			uploadCookiesOpen = false;
			cookieInputText = '';
		} else {
			toast.error('Upload Failed', chatgptState.error || 'Could not save cookies');
		}
	}

	async function handleDeleteCookies() {
		const ok = await chatgptState.deleteCookies();
		if (ok) {
			toast.success('Cookies Cleared', 'ChatGPT session returned to anonymous');
		} else {
			toast.error('Failed to Clear', chatgptState.error || 'Unknown error');
		}
	}

	async function handleToggleShell(checked: boolean) {
		const ok = await chatgptState.setShellEnabled(checked);
		if (ok) {
			toast.success(
				checked ? 'Shell Tool Enabled' : 'Shell Tool Disabled',
				checked
					? 'The model can now run local commands for this machine'
					: 'Only read-only file tools remain available'
			);
		} else {
			toast.error('Failed to update tools', chatgptState.error || 'Unknown error');
		}
	}

	async function handleTogglePassthrough(checked: boolean) {
		const ok = await chatgptState.setPassthroughTools(checked);
		if (ok) {
			toast.success(
				checked ? 'Agent Tool Routing Enabled' : 'Router-side Tools Enabled',
				checked
					? 'Tool calls are handed to your agent, which shows each step'
					: 'The router executes tools itself; steps will not appear in your agent'
			);
		} else {
			toast.error('Failed to update tool routing', chatgptState.error || 'Unknown error');
		}
	}

	async function handleSaveWorkDir() {
		const ok = await chatgptState.setWorkDir(workDirInput.trim());
		if (ok) {
			toast.success('Work Directory Saved', chatgptState.workDir || 'default');
		} else {
			toast.error('Failed to save directory', chatgptState.error || 'Unknown error');
		}
	}

	function handleFileSelected(e: Event) {
		const target = e.target as HTMLInputElement;
		const file = target.files?.[0];
		if (!file) return;

		const reader = new FileReader();
		reader.onload = (event) => {
			const text = event.target?.result as string;
			if (text) {
				cookieInputText = text;
			}
		};
		reader.readAsText(file);
	}
</script>

<div class="space-y-6">
	<!-- Header -->
	<div class="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
		<div>
			<div class="flex items-center gap-2.5">
				<div class="flex size-9 items-center justify-center rounded-lg border border-primary/20 bg-primary/10 text-primary">
					<BotIcon class="size-5" />
				</div>
				<div>
					<h1 class="text-xl font-bold tracking-tight">ChatGPT Web</h1>
					<p class="text-xs text-muted-foreground">
						Local headless browser automation for ChatGPT with real-time memory monitoring
					</p>
				</div>
			</div>
		</div>

		<div class="flex items-center gap-3">
			<Badge variant={chatgptState.enabled ? 'default' : 'secondary'} class="gap-1.5 px-2.5 py-1 text-xs">
				<span class={cn('size-1.5 rounded-full', chatgptState.enabled ? 'bg-primary-foreground' : 'bg-muted-foreground')}></span>
				{chatgptState.enabled ? 'Enabled' : 'Disabled (Default)'}
			</Badge>

			<div class="flex items-center gap-2 rounded-lg border border-border bg-card px-3 py-1.5">
				<span class="text-xs font-medium">Provider:</span>
				<Switch
					checked={chatgptState.enabled}
					disabled={chatgptState.toggling || chatgptState.installing}
					onCheckedChange={handleToggleMain}
				/>
			</div>
		</div>
	</div>

	<!-- Status Grid: Stealth Browser, Auth Cookies, & Real Memory Usage -->
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
						{chatgptState.installed ? 'Installed' : 'Not Installed'}
					</span>
					<Badge variant={chatgptState.installed ? 'outline' : 'destructive'} class="text-xs">
						{chatgptState.installed ? 'Ready' : 'Download Needed'}
					</Badge>
				</div>
				<p class="mt-1 truncate text-xs text-muted-foreground">
					{chatgptState.browserPath || 'No browser binary detected in data directory'}
				</p>

				{#if !chatgptState.installed && !chatgptState.installing}
					<Button
						variant="outline"
						size="sm"
						class="mt-3 w-full gap-1.5 text-xs"
						onclick={() => (confirmInstallOpen = true)}
					>
						<DownloadIcon class="size-3.5" />
						Download Stealth Browser (~150 MB)
					</Button>
				{/if}
			</Card.Content>
		</Card.Root>

		<!-- Cookies Authentication Card -->
		<Card.Root>
			<Card.Header class="pb-2 flex flex-row items-center justify-between space-y-0">
				<Card.Title class="flex items-center gap-2 text-sm font-medium text-muted-foreground">
					<KeyRoundIcon class="size-4" />
					Authentication Cookies
				</Card.Title>
			</Card.Header>
			<Card.Content>
				<div class="flex items-baseline justify-between">
					<span class="text-xl font-bold">
						{chatgptState.hasCookies ? 'Configured' : 'Anonymous'}
					</span>
					<Badge variant={chatgptState.hasCookies ? 'default' : 'secondary'} class="text-xs">
						{chatgptState.hasCookies ? 'Logged In' : 'No Auth'}
					</Badge>
				</div>
				<p class="mt-1 text-xs text-muted-foreground">
					{chatgptState.hasCookies ? 'Using custom session cookies' : 'Running as unauthenticated guest'}
				</p>

				<div class="mt-3 flex gap-2">
					<Button
						variant="outline"
						size="sm"
						class="flex-1 gap-1.5 text-xs"
						onclick={() => (uploadCookiesOpen = true)}
					>
						<UploadIcon class="size-3.5" />
						Upload Cookies
					</Button>
					{#if chatgptState.hasCookies}
						<Button
							variant="ghost"
							size="sm"
							class="text-destructive hover:bg-destructive/10 text-xs px-2.5"
							onclick={handleDeleteCookies}
							title="Clear cookies"
						>
							<Trash2Icon class="size-3.5" />
						</Button>
					{/if}
				</div>
			</Card.Content>
		</Card.Root>

		<!-- Real Browser RAM Usage Card (Real-time polling) -->
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
						{#if chatgptState.browserRunning && chatgptState.browserRssMb > 0}
							{chatgptState.browserRssMb.toFixed(1)} MB
						{:else if chatgptState.enabled}
							0.0 MB <span class="text-xs font-normal text-muted-foreground">(Idle)</span>
						{:else}
							0.0 MB <span class="text-xs font-normal text-muted-foreground">(Stopped)</span>
						{/if}
					</div>
					<Badge variant={chatgptState.browserRunning ? 'default' : 'outline'} class="text-xs font-mono">
						{#if chatgptState.browserRunning}
							{chatgptState.browserProcs} procs (PID {chatgptState.browserPid})
						{:else}
							0 procs
						{/if}
					</Badge>
				</div>
				<p class="mt-1 text-xs text-muted-foreground">
					Summed resident memory (RSS) refreshed every 2 seconds.
				</p>
			</Card.Content>
		</Card.Root>
	</div>

	<!-- Tool Access Card -->
	<Card.Root>
		<Card.Header class="pb-2">
			<Card.Title class="flex items-center gap-2 text-sm font-medium text-muted-foreground">
				<WrenchIcon class="size-4" />
				Tool Access
			</Card.Title>
			<Card.Description class="pt-1 text-xs">
				ChatGPT web cannot emit native tool calls, so the router runs an internal tool loop.
				Read-only tools (read, list, grep, glob) are always on and the project directory is
				auto-detected per request. Shell is opt-in.
			</Card.Description>
		</Card.Header>
		<Card.Content class="space-y-4">
			<div class="flex items-center justify-between rounded-lg border border-primary/30 bg-primary/5 p-3">
				<div class="min-w-0 pe-3">
					<p class="text-sm font-medium">Hand tool calls to the agent</p>
					<p class="text-xs text-muted-foreground">
						Recommended. The router returns OpenAI <code>tool_calls</code> so your agent
						(Pi, OpenCode) displays each step and runs the tool with its own sandbox. Turn off
						to have the router execute tools internally — results are instant but no steps appear.
					</p>
				</div>
				<Switch
					checked={chatgptState.passthroughTools}
					onCheckedChange={handleTogglePassthrough}
				/>
			</div>

			<div class="flex items-center justify-between rounded-lg border border-border p-3">
				<div class="min-w-0 pe-3">
					<p class="text-sm font-medium">Shell execution</p>
					<p class="text-xs text-muted-foreground">
						Runs bash on this machine on behalf of the model. Enables test and build commands.
						Only enable for trusted API keys.
					</p>
				</div>
				<Switch
					checked={chatgptState.shellEnabled}
					onCheckedChange={handleToggleShell}
				/>
			</div>

			<div class="space-y-1.5">
				<label for="chatgpt-workdir" class="text-xs font-medium"
					>Work directory override <span class="text-muted-foreground">(optional)</span></label
				>
				<div class="flex gap-2">
					<input
						id="chatgpt-workdir"
						bind:value={workDirInput}
						placeholder={chatgptState.workDir || 'auto-detected per request'}
						class="h-9 flex-1 rounded-md border border-input bg-muted/40 px-2.5 font-mono text-xs focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
					/>
					<Button variant="outline" size="sm" class="text-xs" onclick={handleSaveWorkDir}>
						Save
					</Button>
				</div>
				<p class="text-xs text-muted-foreground">
					Normally not needed. The router detects the project per request from the client's
					<code>cwd</code>/working-directory hint, from absolute paths in the prompt, or from request
					metadata. Set this only to force a fixed directory.
					Current: <span class="font-mono">{chatgptState.workDir || '(auto-detect)'}</span>
				</p>
			</div>
		</Card.Content>
	</Card.Root>

	<!-- Installation Progress Card (when installing) -->
	{#if chatgptState.installing || chatgptState.installState === 'downloading' || chatgptState.installState === 'extracting'}
		<Card.Root class="border-primary/30 bg-primary/5">
			<Card.Content class="pt-6">
				<div class="flex items-center justify-between">
					<div class="flex items-center gap-2">
						<RefreshCwIcon class="size-4 animate-spin text-primary" />
						<span class="text-sm font-medium">
							{chatgptState.installState === 'downloading'
								? `Downloading Stealth Chromium (${chatgptState.installProgress}%)...`
								: 'Extracting and verifying browser binary...'}
						</span>
					</div>
					<span class="text-xs font-mono text-muted-foreground">{chatgptState.installProgress}%</span>
				</div>
				<div class="mt-3 h-2 w-full overflow-hidden rounded-full bg-secondary">
					<div
						class="h-full bg-primary transition-all duration-300"
						style={`width: ${chatgptState.installProgress}%`}
					></div>
				</div>
			</Card.Content>
		</Card.Root>
	{/if}

	<!-- Model Test Chat Section -->
	<ModelTestChat models={providerModels} bind:selectedModelId={selectedChatModel} />

	<!-- Models Table Card -->
	<Card.Root>
		<Card.Header>
			<Card.Title class="text-base font-semibold">Available Models</Card.Title>
			<Card.Description>
				Models accessible through the standard OpenAI API at <code>/v1/chat/completions</code> when enabled.
			</Card.Description>
		</Card.Header>
		<Card.Content>
			<Table.Root>
				<Table.Header>
					<Table.Row>
						<Table.Head class="w-[240px]">Model ID</Table.Head>
						<Table.Head>Description</Table.Head>
						<Table.Head class="w-[120px]">Status</Table.Head>
					</Table.Row>
				</Table.Header>
				<Table.Body>
					{#each rawModels as m (m.id)}
						<Table.Row>
							<Table.Cell class="font-mono text-xs font-medium">
								{m.id}
							</Table.Cell>
							<Table.Cell class="text-xs text-muted-foreground">
								{m.desc}
							</Table.Cell>
							<Table.Cell>
								<Badge variant={chatgptState.enabled ? 'outline' : 'secondary'} class="text-xs">
									{chatgptState.enabled ? 'Active' : 'Disabled'}
								</Badge>
							</Table.Cell>
						</Table.Row>
					{/each}
				</Table.Body>
			</Table.Root>
		</Card.Content>
	</Card.Root>
</div>

<!-- Upload Cookies Dialog -->
<Dialog.Root bind:open={uploadCookiesOpen}>
	<Dialog.Content class="sm:max-w-lg">
		<Dialog.Header>
			<div class="flex items-center gap-2 text-primary">
				<KeyRoundIcon class="size-5" />
				<Dialog.Title>Upload ChatGPT Cookies</Dialog.Title>
			</div>
			<Dialog.Description class="pt-2 text-xs leading-relaxed text-muted-foreground">
				Upload a JSON array of cookies exported from your browser (e.g. via browser cookie export extensions).
				This enables access to logged-in models and accounts.
			</Dialog.Description>
		</Dialog.Header>

		<div class="space-y-4 py-2">
			<div>
				<div class="flex items-center justify-between pb-1.5">
					<label for="cookie-json-area" class="text-xs font-medium">Cookie JSON</label>
					<Button
						variant="outline"
						size="sm"
						class="h-7 text-xs gap-1.5"
						onclick={() => fileInputRef?.click()}
					>
						<UploadIcon class="size-3" />
						Import File (.json)
					</Button>
					<input
						type="file"
						accept=".json,application/json"
						class="hidden"
						bind:this={fileInputRef}
						onchange={handleFileSelected}
					/>
				</div>
				<textarea
					id="cookie-json-area"
					bind:value={cookieInputText}
					placeholder={`[\n  {\n    "name": "__Secure-next-auth.session-token",\n    "value": "...",\n    "domain": "chatgpt.com"\n  }\n]`}
					class="h-44 w-full rounded-md border border-input bg-muted/40 p-2.5 font-mono text-xs focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
				></textarea>
			</div>
		</div>

		<Dialog.Footer class="gap-2 sm:gap-0">
			<Button variant="outline" size="sm" onclick={() => (uploadCookiesOpen = false)}>
				Cancel
			</Button>
			<Button
				size="sm"
				disabled={chatgptState.uploadingCookies || !cookieInputText.trim()}
				onclick={handleUploadCookiesSubmit}
			>
				{chatgptState.uploadingCookies ? 'Saving...' : 'Save Cookies'}
			</Button>
		</Dialog.Footer>
	</Dialog.Content>
</Dialog.Root>

<!-- Download Confirmation Dialog -->
<Dialog.Root bind:open={confirmInstallOpen}>
	<Dialog.Content class="sm:max-w-md">
		<Dialog.Header>
			<div class="flex items-center gap-2 text-primary">
				<DownloadIcon class="size-5" />
				<Dialog.Title>Download Stealth Browser</Dialog.Title>
			</div>
			<Dialog.Description class="pt-2 text-xs leading-relaxed text-muted-foreground">
				ChatGPT Web requires a headless stealth Chromium build (~150 MB download) to solve Turnstile challenges and mint required security tokens locally.
				<br /><br />
				The download will be stored in your application data directory. Once finished, ChatGPT Web will be automatically enabled.
			</Dialog.Description>
		</Dialog.Header>

		{#if chatgptState.installing}
			<div class="space-y-3 py-4">
				<div class="flex items-center justify-between text-xs">
					<span>{chatgptState.installState === 'downloading' ? 'Downloading...' : 'Extracting...'}</span>
					<span class="font-mono">{chatgptState.installProgress}%</span>
				</div>
				<div class="h-2 w-full overflow-hidden rounded-full bg-secondary">
					<div
						class="h-full bg-primary transition-all duration-300"
						style={`width: ${chatgptState.installProgress}%`}
					></div>
				</div>
			</div>
		{:else}
			<Dialog.Footer class="gap-2 sm:gap-0">
				<Button variant="outline" size="sm" onclick={() => (confirmInstallOpen = false)}>
					Cancel
				</Button>
				<Button size="sm" onclick={handleConfirmDownload}>
					Download Now
				</Button>
			</Dialog.Footer>
		{/if}
	</Dialog.Content>
</Dialog.Root>
