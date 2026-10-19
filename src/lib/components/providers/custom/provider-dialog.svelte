<script lang="ts">
	import * as Dialog from '$lib/components/ui/dialog';
	import * as Alert from '$lib/components/ui/alert';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Switch } from '$lib/components/ui/switch';
	import { Badge } from '$lib/components/ui/badge';
	import type {
		CustomProvider,
		CreateCustomProviderPayload,
		UpdateCustomProviderPayload
	} from '$lib/types';

	import ServerIcon from '@lucide/svelte/icons/server';
	import AlertCircleIcon from '@lucide/svelte/icons/alert-circle';
	import EyeIcon from '@lucide/svelte/icons/eye';
	import EyeOffIcon from '@lucide/svelte/icons/eye-off';

	interface Props {
		open: boolean;
		provider?: CustomProvider | null;
		onSave: (payload: CreateCustomProviderPayload | UpdateCustomProviderPayload) => Promise<void>;
		onClose: () => void;
	}

	let { open = $bindable(false), provider = null, onSave, onClose }: Props = $props();

	let name = $state('');
	let prefix = $state('');
	let baseUrl = $state('');
	let apiKey = $state('');
	let isEnabled = $state(true);
	let showApiKey = $state(false);
	let saving = $state(false);
	let error = $state<string | null>(null);

	let isEditing = $derived(!!provider);

	function resetForm() {
		if (provider) {
			name = provider.name;
			prefix = provider.prefix;
			baseUrl = provider.baseUrl;
			apiKey = '';
			isEnabled = provider.isEnabled;
		} else {
			name = '';
			prefix = '';
			baseUrl = '';
			apiKey = '';
			isEnabled = true;
		}
		error = null;
		showApiKey = false;
	}

	function handleNameChange(e: Event) {
		const target = e.target as HTMLInputElement;
		name = target.value;
		if (
			!isEditing &&
			(!prefix ||
				prefix ===
					name
						.slice(0, -1)
						.toLowerCase()
						.replace(/[^a-z0-9_-]/g, ''))
		) {
			prefix = name.toLowerCase().replace(/[^a-z0-9_-]/g, '');
		}
	}

	function handlePrefixInput(e: Event) {
		const target = e.target as HTMLInputElement;
		prefix = target.value.toLowerCase().replace(/[^a-z0-9_-]/g, '');
	}

	async function handleSubmit(e: SubmitEvent) {
		e.preventDefault();
		const trimmedName = name.trim();
		const trimmedPrefix = prefix.trim().toLowerCase();
		const trimmedBaseUrl = baseUrl.trim();

		if (!trimmedName) {
			error = 'Provider name is required';
			return;
		}
		if (!trimmedPrefix) {
			error = 'Model prefix is required';
			return;
		}
		if (!trimmedBaseUrl) {
			error = 'Base URL is required';
			return;
		}
		if (!trimmedBaseUrl.startsWith('http://') && !trimmedBaseUrl.startsWith('https://')) {
			error = 'Base URL must start with http:// or https://';
			return;
		}

		try {
			saving = true;
			error = null;

			if (isEditing) {
				const payload: UpdateCustomProviderPayload = {
					name: trimmedName,
					prefix: trimmedPrefix,
					baseUrl: trimmedBaseUrl,
					isEnabled
				};
				if (apiKey.trim() !== '') {
					payload.apiKey = apiKey.trim();
				}
				await onSave(payload);
			} else {
				const payload: CreateCustomProviderPayload = {
					name: trimmedName,
					prefix: trimmedPrefix,
					baseUrl: trimmedBaseUrl,
					apiKey: apiKey.trim(),
					isEnabled
				};
				await onSave(payload);
			}
			open = false;
			onClose();
		} catch (e: unknown) {
			error = e instanceof Error ? e.message : 'Failed to save provider';
		} finally {
			saving = false;
		}
	}
</script>

<Dialog.Root
	bind:open
	onOpenChange={(val) => {
		if (val) {
			resetForm();
		} else {
			onClose();
		}
	}}
>
	<Dialog.Content class="sm:max-w-lg">
		<Dialog.Header>
			<Dialog.Title class="flex items-center gap-2 text-base font-semibold">
				<ServerIcon class="size-4 text-primary" />
				<span>{isEditing ? 'Edit Custom Provider' : 'Add Custom Provider'}</span>
			</Dialog.Title>
			<Dialog.Description class="text-xs">
				Configure an OpenAI-compatible upstream provider. You can set the name and model prefix for
				request routing.
			</Dialog.Description>
		</Dialog.Header>

		{#if error}
			<Alert.Root variant="destructive" class="py-2.5">
				<AlertCircleIcon class="size-4" />
				<Alert.Title class="text-xs font-medium">Error</Alert.Title>
				<Alert.Description class="text-xs">{error}</Alert.Description>
			</Alert.Root>
		{/if}

		<form onsubmit={handleSubmit} class="space-y-4 pt-1">
			<!-- Name -->
			<div class="space-y-1.5">
				<label for="provider-name" class="text-xs font-medium text-foreground">
					Provider Name <span class="text-destructive">*</span>
				</label>
				<Input
					id="provider-name"
					type="text"
					placeholder="e.g. Groq, DeepSeek, Together AI, Ollama"
					value={name}
					oninput={handleNameChange}
					required
					disabled={saving}
				/>
			</div>

			<!-- Prefix -->
			<div class="space-y-1.5">
				<div class="flex items-center justify-between">
					<label for="provider-prefix" class="text-xs font-medium text-foreground">
						Model Prefix <span class="text-destructive">*</span>
					</label>
					{#if prefix}
						<Badge variant="outline" class="font-mono text-[10px]">
							{prefix}/&lt;model&gt;
						</Badge>
					{/if}
				</div>
				<Input
					id="provider-prefix"
					type="text"
					placeholder="e.g. groq, deepseek, together, ollama"
					value={prefix}
					oninput={handlePrefixInput}
					required
					disabled={saving}
				/>
				<p class="text-[11px] text-muted-foreground">
					Routing namespace: models will be addressed as <span
						class="font-mono font-medium text-foreground"
						>{prefix ? `${prefix}/model-name` : 'prefix/model-name'}</span
					>.
				</p>
			</div>

			<!-- Base URL -->
			<div class="space-y-1.5">
				<label for="provider-url" class="text-xs font-medium text-foreground">
					OpenAI Base URL <span class="text-destructive">*</span>
				</label>
				<Input
					id="provider-url"
					type="url"
					placeholder="https://api.groq.com/openai/v1 or http://localhost:11434/v1"
					bind:value={baseUrl}
					required
					disabled={saving}
				/>
				<p class="text-[11px] text-muted-foreground">
					Root endpoint containing <span class="font-mono text-[11px]">/chat/completions</span> and
					<span class="font-mono text-[11px]">/models</span>.
				</p>
			</div>

			<!-- API Key -->
			<div class="space-y-1.5">
				<div class="flex items-center justify-between">
					<label for="provider-key" class="text-xs font-medium text-foreground"> API Key </label>
					<span class="text-[10px] text-muted-foreground">
						{isEditing && provider?.hasApiKey
							? 'Leave blank to keep existing'
							: 'Optional for local models'}
					</span>
				</div>
				<div class="relative">
					<Input
						id="provider-key"
						type={showApiKey ? 'text' : 'password'}
						placeholder={isEditing && provider?.hasApiKey
							? '••••••••••••••••'
							: 'sk-... or leave empty for Ollama'}
						bind:value={apiKey}
						disabled={saving}
						class="pe-9"
					/>
					<Button
						type="button"
						variant="ghost"
						size="icon"
						class="absolute end-0 top-0 h-full w-9 text-muted-foreground hover:text-foreground"
						onclick={() => (showApiKey = !showApiKey)}
						tabindex={-1}
					>
						{#if showApiKey}
							<EyeOffIcon class="size-4" />
						{:else}
							<EyeIcon class="size-4" />
						{/if}
					</Button>
				</div>
			</div>

			<!-- Enabled Switch -->
			<div class="flex items-center justify-between rounded-lg border border-border p-3">
				<div class="flex flex-col gap-0.5">
					<span class="text-xs font-medium text-foreground">Enable Provider</span>
					<span class="text-[11px] text-muted-foreground">
						Route chat completion requests and advertise models in /v1/models
					</span>
				</div>
				<Switch bind:checked={isEnabled} disabled={saving} />
			</div>

			<Dialog.Footer class="pt-2">
				<Button type="button" variant="outline" size="sm" onclick={onClose} disabled={saving}>
					Cancel
				</Button>
				<Button type="submit" size="sm" disabled={saving}>
					{saving ? 'Saving...' : isEditing ? 'Save Changes' : 'Add Provider'}
				</Button>
			</Dialog.Footer>
		</form>
	</Dialog.Content>
</Dialog.Root>
