import type { PonytailModeInfo, PonytailStats, PonytailTestResult } from '$lib/types';
import {
	fetchPonytailStatus,
	updatePonytailConfig,
	syncPonytailUpstream,
	testPonytailPrompt
} from '$lib/api/ponytail';

export class PonytailState {
	enabled = $state(true);
	mode = $state('full');
	autoUpdate = $state(true);
	lastSync = $state<string | null>(null);
	upstreamCommit = $state('');
	upstreamCommitMsg = $state('');
	upstreamCommitDate = $state('');
	upstreamURL = $state('https://github.com/DietrichGebert/ponytail');
	activePrompt = $state('');
	availableModes = $state<PonytailModeInfo[]>([
		{
			id: 'full',
			name: 'Full (Default)',
			description:
				'The smallest complete change that fully works. YAGNI, standard library first, zero unasked abstractions.'
		},
		{
			id: 'lite',
			name: 'Lite',
			description:
				'Builds what was requested, but names the smaller lazy option in one line for the user to choose.'
		},
		{
			id: 'ultra',
			name: 'Ultra (Pushback)',
			description:
				'Questions requests and pushes back on speculative complexity before writing a line of code.'
		},
		{
			id: 'review',
			name: 'Review',
			description:
				'Senior developer code review: correctness, safety, load resilience, lean diffs, and zero bloat.'
		}
	]);
	customPrompt = $state('');
	stats = $state<PonytailStats>({
		total_requests: 0,
		injected_requests: 0
	});

	loading = $state(false);
	syncing = $state(false);
	toggling = $state(false);
	testing = $state(false);
	error = $state<string | null>(null);
	testResult = $state<PonytailTestResult | null>(null);

	async load() {
		try {
			this.loading = true;
			this.error = null;
			const data = await fetchPonytailStatus();
			this.enabled = data.enabled;
			this.mode = data.mode;
			this.autoUpdate = data.auto_update;
			this.lastSync = data.last_sync;
			this.upstreamCommit = data.upstream_commit;
			this.upstreamCommitMsg = data.upstream_commit_msg;
			this.upstreamCommitDate = data.upstream_commit_date;
			this.upstreamURL = data.upstream_url;
			this.activePrompt = data.active_prompt;
			if (data.available_modes) {
				this.availableModes = data.available_modes;
			}
			this.customPrompt = data.custom_prompt || '';
			this.stats = data.stats;
		} catch (e: unknown) {
			this.error = e instanceof Error ? e.message : 'Failed to load Ponytail status';
		} finally {
			this.loading = false;
		}
	}

	async toggleEnabled() {
		try {
			this.toggling = true;
			this.error = null;
			const next = !this.enabled;
			const data = await updatePonytailConfig({ enabled: next });
			this.enabled = data.enabled;
			this.stats = data.stats;
		} catch (e: unknown) {
			this.error = e instanceof Error ? e.message : 'Failed to update Ponytail toggle';
		} finally {
			this.toggling = false;
		}
	}

	async setMode(newMode: string) {
		try {
			this.loading = true;
			this.error = null;
			const data = await updatePonytailConfig({ mode: newMode });
			this.mode = data.mode;
			this.activePrompt = data.active_prompt;
		} catch (e: unknown) {
			this.error = e instanceof Error ? e.message : 'Failed to change Ponytail mode';
		} finally {
			this.loading = false;
		}
	}

	async syncUpstream() {
		try {
			this.syncing = true;
			this.error = null;
			const res = await syncPonytailUpstream();
			if (res.status) {
				const data = res.status;
				this.enabled = data.enabled;
				this.mode = data.mode;
				this.lastSync = data.last_sync;
				this.upstreamCommit = data.upstream_commit;
				this.upstreamCommitMsg = data.upstream_commit_msg;
				this.upstreamCommitDate = data.upstream_commit_date;
				this.activePrompt = data.active_prompt;
				this.stats = data.stats;
			}
			return res.updated;
		} catch (e: unknown) {
			this.error =
				e instanceof Error ? e.message : 'Failed to sync with upstream Ponytail repository';
			throw e;
		} finally {
			this.syncing = false;
		}
	}

	async runTest(userPrompt: string, provider?: string, model?: string, systemPrior?: string) {
		try {
			this.testing = true;
			this.error = null;
			const res = await testPonytailPrompt({
				provider: provider || 'opencode',
				model: model || 'muse-spark-1.3-contributor-free',
				mode: this.mode,
				user_prompt: userPrompt,
				system_prior: systemPrior
			});
			this.testResult = res;
		} catch (e: unknown) {
			this.error = e instanceof Error ? e.message : 'Failed to run Ponytail test prompt';
		} finally {
			this.testing = false;
		}
	}
}

export const ponytailState = new PonytailState();
