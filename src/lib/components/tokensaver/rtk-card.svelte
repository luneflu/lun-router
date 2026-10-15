<script lang="ts">
	import * as Card from '$lib/components/ui/card';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { Switch } from '$lib/components/ui/switch';
	import { rtkState } from '$lib/state/rtk.svelte';
	import ZapIcon from '@lucide/svelte/icons/zap';
	import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';
	import ArrowRightIcon from '@lucide/svelte/icons/arrow-right';
	import CheckCircle2Icon from '@lucide/svelte/icons/check-circle-2';
	import PlayIcon from '@lucide/svelte/icons/play';
	import SparklesIcon from '@lucide/svelte/icons/sparkles';
	import CopyIcon from '@lucide/svelte/icons/copy';
	import CheckIcon from '@lucide/svelte/icons/check';

	let sampleType = $state<
		'diff' | 'status' | 'grep' | 'find' | 'build' | 'cargo' | 'pytest' | 'gotest'
	>('diff');
	let testInput = $state(getSample('diff'));
	let copied = $state(false);

	function formatBytes(bytes: number): string {
		if (bytes >= 1048576) return `${(bytes / 1048576).toFixed(1)} MB`;
		if (bytes >= 1024) return `${(bytes / 1024).toFixed(1)} KB`;
		return `${bytes} B`;
	}

	function getSample(
		type: 'diff' | 'status' | 'grep' | 'find' | 'build' | 'cargo' | 'pytest' | 'gotest'
	): string {
		if (type === 'diff') {
			let diff = `diff --git a/src/app.js b/src/app.js\nindex 1234abc..5678def 100644\n--- a/src/app.js\n+++ b/src/app.js\n@@ -1,5 +1,180 @@\n`;
			for (let i = 1; i <= 150; i++) {
				diff += `+const config_${i} = { timeout: 3000, enabled: true, debug: false, retryCount: 3 };\n`;
			}
			return diff.trimEnd();
		}
		if (type === 'status') {
			return `On branch main\nYour branch is up to date with 'origin/main'.\n\nChanges not staged for commit:\n  (use "git add <file>..." to update what will be committed)\n\tmodified:   src/a.js\n\tmodified:   src/b.js\n\tmodified:   src/components/card.svelte\n\tdeleted:    src/old.js\n\nUntracked files:\n\tnotes.txt\n\ttemp.log\n\nno changes added to commit`;
		}
		if (type === 'grep') {
			let g = '';
			for (let i = 1; i <= 30; i++) {
				g += `src/handler/auth.go:${i}:const sessionToken${i} = "Bearer token-data-placeholder-long-padding"\n`;
			}
			for (let i = 1; i <= 15; i++) {
				g += `src/store/user.go:${i}:func ValidateUser${i}() bool { return true }\n`;
			}
			return g.trimEnd();
		}
		if (type === 'find') {
			let f = '';
			for (let i = 0; i < 25; i++) {
				f += `./internal/handler/router_${i}.go\n`;
			}
			for (let i = 0; i < 15; i++) {
				f += `./src/lib/components/ui/button_${i}.svelte\n`;
			}
			return f.trimEnd();
		}
		if (type === 'cargo') {
			return `   Compiling myapp v0.1.0\n    Finished test [unoptimized + debuginfo] in 0.82s\n     Running unittests src/lib.rs\nrunning 35 tests\ntest tests::test_1 ... ok\ntest tests::test_2 ... ok\ntest tests::test_fail ... FAILED\n\nfailures:\n\n---- tests::test_fail stdout ----\nthread 'main' panicked at 'assertion failed: 1 == 2', src/lib.rs:42:9\n\nfailures:\n    tests::test_fail\n\ntest result: FAILED. 34 passed; 1 failed; 0 ignored; 0 measured; 0 filtered out`;
		}
		if (type === 'pytest') {
			return `============================= test session starts ==============================\nplatform darwin -- Python 3.11.0\ncollected 45 items\n\ntests/test_app.py .........................F.................            [100%]\n\n=================================== FAILURES ===================================\n__________________________________ test_fail ___________________________________\n    def test_fail():\n>       assert 1 == 2\nE       assert 1 == 2\n\ntests/test_app.py:5: AssertionError\n=========================== short test summary info ============================\nFAILED tests/test_app.py::test_fail - assert 1 == 2\n========================= 1 failed, 44 passed in 0.45s =========================`;
		}
		if (type === 'gotest') {
			return `=== RUN   TestLoginSuccess\n--- PASS: TestLoginSuccess (0.01s)\n=== RUN   TestLogout\n--- PASS: TestLogout (0.00s)\n=== RUN   TestTokenExpiry\n--- FAIL: TestTokenExpiry (0.02s)\n    auth_test.go:58: token expired earlier than expected\nFAIL\nFAIL\tapp/auth\t0.035s\n=== RUN   TestDatabasePing\n--- PASS: TestDatabasePing (0.00s)\nPASS\nok  \tapp/db\t0.012s`;
		}
		return `Compiling foo v0.1.0\nCompiling bar v0.2.0\nCompiling baz v0.3.0\nDownloading serde v1.0.1\nDownloading tokio v1.2.0\nwarning: unused import: 'std::collections::HashMap'\n --> src/main.rs:2:5\n  |\n2 | use std::collections::HashMap;\n  |     ^^^^^^^^^^^^^^^^^^^^^^^^^\nFinished release [optimized] target(s) in 1.42s\nBUILD SUCCESS`;
	}

	function handleSelectSample(
		type: 'diff' | 'status' | 'grep' | 'find' | 'build' | 'cargo' | 'pytest' | 'gotest'
	) {
		sampleType = type;
		testInput = getSample(type);
		rtkState.testResult = null;
	}

	async function handleRunTest() {
		await rtkState.runTest(testInput);
	}

	async function handleCopy() {
		if (!rtkState.testResult?.output) return;
		await navigator.clipboard.writeText(rtkState.testResult.output);
		copied = true;
		setTimeout(() => (copied = false), 2000);
	}
</script>

<div class="flex flex-col gap-6">
	<!-- Main RTK Card -->
	<Card.Root class="overflow-hidden border-border bg-card">
		<Card.Header class="border-b border-border/40 pb-4">
			<div class="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
				<div class="flex items-center gap-3">
					<div
						class="flex size-10 items-center justify-center rounded-lg border border-amber-500/20 bg-amber-500/10 text-amber-500 shadow-xs"
					>
						<ZapIcon class="size-5" />
					</div>
					<div>
						<div class="flex items-center gap-2">
							<Card.Title class="text-base font-semibold">RTK Token Saver</Card.Title>
							<Badge variant={rtkState.rtkEnabled ? 'default' : 'outline'} class="text-[10px]">
								{rtkState.rtkEnabled ? 'Active' : 'Disabled'}
							</Badge>
							<Badge variant="secondary" class="text-[10px]">Embedded Go (Fast)</Badge>
							{#if rtkState.systemRtkAvailable}
								<Badge variant="outline" class="border-emerald-500/30 text-[10px] text-emerald-500">
									CLI RTK Detected
								</Badge>
							{/if}
						</div>
						<Card.Description class="text-xs">
							Auto-compress tool outputs (<code class="text-[11px]">git diff</code>,
							<code class="text-[11px]">grep</code>, <code class="text-[11px]">ls</code>,
							<code class="text-[11px]">tree</code>, build logs) before sending to LLMs.
						</Card.Description>
					</div>
				</div>

				<div class="flex items-center gap-3 self-end sm:self-auto">
					<Button
						variant="outline"
						size="sm"
						disabled={rtkState.loading}
						onclick={() => rtkState.load()}
						class="h-8 text-xs"
					>
						<RefreshCwIcon class={`size-3.5 ${rtkState.loading ? 'animate-spin' : ''}`} />
						Refresh
					</Button>

					<div class="flex items-center gap-2 border-s border-border ps-3">
						<span class="text-xs font-medium text-muted-foreground">Enable RTK</span>
						<Switch
							checked={rtkState.rtkEnabled}
							disabled={rtkState.toggling}
							onCheckedChange={() => rtkState.toggleRTK()}
						/>
					</div>
				</div>
			</div>
		</Card.Header>

		<Card.Content class="flex flex-col gap-6 pt-5">
			<!-- Metrics row -->
			<div class="grid grid-cols-2 gap-3 sm:grid-cols-4">
				<div class="rounded-lg border border-border/60 bg-muted/20 p-3.5">
					<span class="text-[11px] font-medium text-muted-foreground">Requests Processed</span>
					<div class="mt-1 flex items-baseline gap-2">
						<span class="text-2xl font-bold tracking-tight">{rtkState.stats.total_requests}</span>
					</div>
					<span class="text-[10px] text-muted-foreground">Total completion queries</span>
				</div>

				<div class="rounded-lg border border-border/60 bg-muted/20 p-3.5">
					<span class="text-[11px] font-medium text-muted-foreground">Compressed</span>
					<div class="mt-1 flex items-baseline gap-2">
						<span class="text-2xl font-bold tracking-tight text-amber-500"
							>{rtkState.stats.compressed_requests}</span
						>
					</div>
					<span class="text-[10px] text-muted-foreground">Tool outputs compacted</span>
				</div>

				<div class="rounded-lg border border-border/60 bg-muted/20 p-3.5">
					<span class="text-[11px] font-medium text-muted-foreground">Bytes Saved</span>
					<div class="mt-1 flex items-baseline gap-2">
						<span class="text-2xl font-bold tracking-tight text-emerald-500"
							>{formatBytes(rtkState.stats.saved_bytes)}</span
						>
					</div>
					<span class="text-[10px] text-muted-foreground">Bandwidth & context cut</span>
				</div>

				<div class="rounded-lg border border-border/60 bg-muted/20 p-3.5">
					<span class="text-[11px] font-medium text-muted-foreground">Token Efficiency</span>
					<div class="mt-1 flex items-baseline gap-2">
						<span class="text-2xl font-bold tracking-tight text-primary">
							{rtkState.stats.saved_percent > 0
								? `${rtkState.stats.saved_percent.toFixed(1)}%`
								: '0%'}
						</span>
					</div>
					<span class="text-[10px] text-muted-foreground">Average compression ratio</span>
				</div>
			</div>

			<!-- Supported Filters -->
			<div class="flex flex-col gap-2">
				<div class="flex items-center justify-between">
					<span class="text-xs font-semibold tracking-wider text-muted-foreground uppercase">
						Supported Filters ({rtkState.supportedFilters.length})
					</span>
					<span class="text-[11px] text-muted-foreground">
						Bypass header: <code class="rounded bg-muted px-1 py-0.5">X-RTK: off</code>
					</span>
				</div>

				<div class="flex flex-wrap gap-1.5">
					{#each rtkState.supportedFilters as filter (filter)}
						<Badge variant="secondary" class="font-mono text-[10px]">
							{filter}
						</Badge>
					{/each}
				</div>
			</div>
		</Card.Content>
	</Card.Root>

	<!-- Interactive Simulator & Playground -->
	<Card.Root class="border-border bg-card">
		<Card.Header class="pb-3">
			<div class="flex items-center justify-between">
				<div class="flex items-center gap-2">
					<SparklesIcon class="size-4 text-primary" />
					<Card.Title class="text-sm font-semibold">RTK Compression Playground</Card.Title>
				</div>
				<span class="text-xs text-muted-foreground">Test filter detection and token reduction</span>
			</div>
		</Card.Header>

		<Card.Content class="flex flex-col gap-4">
			<!-- Preset sample buttons -->
			<div class="flex flex-wrap items-center gap-2">
				<span class="text-xs font-medium text-muted-foreground">Samples:</span>
				<Button
					variant={sampleType === 'diff' ? 'default' : 'outline'}
					size="sm"
					class="h-7 text-xs"
					onclick={() => handleSelectSample('diff')}
				>
					Git Diff
				</Button>
				<Button
					variant={sampleType === 'status' ? 'default' : 'outline'}
					size="sm"
					class="h-7 text-xs"
					onclick={() => handleSelectSample('status')}
				>
					Git Status
				</Button>
				<Button
					variant={sampleType === 'grep' ? 'default' : 'outline'}
					size="sm"
					class="h-7 text-xs"
					onclick={() => handleSelectSample('grep')}
				>
					Grep
				</Button>
				<Button
					variant={sampleType === 'find' ? 'default' : 'outline'}
					size="sm"
					class="h-7 text-xs"
					onclick={() => handleSelectSample('find')}
				>
					Find
				</Button>
				<Button
					variant={sampleType === 'cargo' ? 'default' : 'outline'}
					size="sm"
					class="h-7 text-xs"
					onclick={() => handleSelectSample('cargo')}
				>
					Cargo Test
				</Button>
				<Button
					variant={sampleType === 'pytest' ? 'default' : 'outline'}
					size="sm"
					class="h-7 text-xs"
					onclick={() => handleSelectSample('pytest')}
				>
					Pytest
				</Button>
				<Button
					variant={sampleType === 'gotest' ? 'default' : 'outline'}
					size="sm"
					class="h-7 text-xs"
					onclick={() => handleSelectSample('gotest')}
				>
					Go Test
				</Button>
				<Button
					variant={sampleType === 'build' ? 'default' : 'outline'}
					size="sm"
					class="h-7 text-xs"
					onclick={() => handleSelectSample('build')}
				>
					Build Output
				</Button>
			</div>

			<!-- Input textarea -->
			<div class="relative">
				<textarea
					bind:value={testInput}
					rows="6"
					placeholder="Paste tool output or command log here..."
					class="w-full rounded-md border border-input bg-muted/20 px-3 py-2 font-mono text-xs shadow-xs focus-visible:ring-1 focus-visible:ring-ring focus-visible:outline-hidden"
				></textarea>
			</div>

			<div class="flex items-center justify-between">
				<span class="text-xs text-muted-foreground">
					Input size: {formatBytes(testInput.length)} ({testInput.length} bytes)
				</span>
				<Button
					size="sm"
					disabled={rtkState.testing || !testInput.trim()}
					onclick={handleRunTest}
					class="h-8 gap-1.5 text-xs"
				>
					<PlayIcon class="size-3.5" />
					Test Compression
				</Button>
			</div>

			<!-- Results display -->
			{#if rtkState.testResult}
				<div class="mt-2 rounded-lg border border-border/80 bg-muted/10 p-4">
					<div class="flex flex-col gap-3">
						<div class="flex flex-wrap items-center justify-between gap-2">
							<div class="flex items-center gap-2">
								<CheckCircle2Icon class="size-4 text-emerald-500" />
								<span class="text-xs font-semibold">Filter Detected:</span>
								{#if rtkState.testResult.filter}
									<Badge variant="default" class="font-mono text-[11px]">
										{rtkState.testResult.filter}
									</Badge>
								{:else}
									<Badge variant="secondary" class="text-[11px]">None (passthrough)</Badge>
								{/if}
							</div>

							<div class="flex items-center gap-3 text-xs">
								<span class="text-muted-foreground">
									{formatBytes(rtkState.testResult.original_size)}
								</span>
								<ArrowRightIcon class="size-3 text-muted-foreground" />
								<span class="font-semibold text-emerald-500">
									{formatBytes(rtkState.testResult.compressed_size)}
								</span>
								<Badge variant="secondary" class="font-bold text-emerald-500">
									-{rtkState.testResult.saved_percent.toFixed(1)}%
								</Badge>
							</div>
						</div>

						<!-- Output preview -->
						<div class="relative mt-1">
							<div class="flex items-center justify-between rounded-t-md bg-muted px-3 py-1.5">
								<span class="text-[10px] font-medium text-muted-foreground">Compressed Output</span>
								<Button
									variant="ghost"
									size="icon-sm"
									class="size-6 text-muted-foreground hover:text-foreground"
									onclick={handleCopy}
									title="Copy output"
								>
									{#if copied}
										<CheckIcon class="size-3.5 text-emerald-500" />
									{:else}
										<CopyIcon class="size-3.5" />
									{/if}
								</Button>
							</div>
							<pre
								class="max-h-60 overflow-y-auto rounded-b-md border border-t-0 border-border/60 bg-muted/40 p-3 font-mono text-[11px] leading-relaxed whitespace-pre">{rtkState
									.testResult.output}</pre>
						</div>
					</div>
				</div>
			{/if}
		</Card.Content>
	</Card.Root>
</div>
