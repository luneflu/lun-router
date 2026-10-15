import type { CavemanModeInfo, CavemanStats, CavemanTestResult } from '$lib/types';
import {
	fetchCavemanStatus,
	updateCavemanConfig,
	syncCavemanUpstream,
	testCavemanPrompt
} from '$lib/api/caveman';

export class CavemanState {
	enabled = $state(true);
	mode = $state('caveman');
	autoUpdate = $state(true);
	lastSync = $state<string | null>(null);
	upstreamCommit = $state('');
	upstreamCommitMsg = $state('');
	upstreamCommitDate = $state('');
	upstreamURL = $state('https://github.com/JuliusBrussee/caveman');
	activePrompt = $state('');
	availableModes = $state<CavemanModeInfo[]>([
		{
			id: 'caveman',
			name: 'Classic Caveman',
			description: 'Answer first, no fluff, every technical fact preserved.',
			tokens_est: 20
		},
		{
			id: 'ultracave',
			name: 'Ultracave',
			description: 'Grammar stripped, fragments only, maximum terseness.',
			tokens_est: 14
		},
		{
			id: 'megacave',
			name: 'Megacave (文言文)',
			description: 'Classical Chinese register, subjects omitted.',
			tokens_est: 13
		}
	]);
	customPrompt = $state('');
	stats = $state<CavemanStats>({
		total_requests: 0,
		injected_requests: 0,
		estimated_saved_pct: 54
	});

	loading = $state(false);
	syncing = $state(false);
	toggling = $state(false);
	testing = $state(false);
	error = $state<string | null>(null);
	testResult = $state<CavemanTestResult | null>(null);

	async load() {
		try {
			this.loading = true;
			this.error = null;
			const data = await fetchCavemanStatus();
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
			this.error = e instanceof Error ? e.message : 'Failed to load Caveman status';
		} finally {
			this.loading = false;
		}
	}

	async toggleEnabled() {
		try {
			this.toggling = true;
			this.error = null;
			const next = !this.enabled;
			const data = await updateCavemanConfig({ enabled: next });
			this.enabled = data.enabled;
			this.stats = data.stats;
		} catch (e: unknown) {
			this.error = e instanceof Error ? e.message : 'Failed to update toggle';
		} finally {
			this.toggling = false;
		}
	}

	async setMode(newMode: string) {
		try {
			this.loading = true;
			this.error = null;
			const data = await updateCavemanConfig({ mode: newMode });
			this.mode = data.mode;
			this.activePrompt = data.active_prompt;
		} catch (e: unknown) {
			this.error = e instanceof Error ? e.message : 'Failed to change Caveman mode';
		} finally {
			this.loading = false;
		}
	}

	async syncUpstream() {
		try {
			this.syncing = true;
			this.error = null;
			const res = await syncCavemanUpstream();
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
				e instanceof Error ? e.message : 'Failed to sync with upstream Caveman repository';
			throw e;
		} finally {
			this.syncing = false;
		}
	}

	async runTest(userPrompt: string, provider?: string, model?: string, systemPrior?: string) {
		try {
			this.testing = true;
			this.error = null;
			const res = await testCavemanPrompt({
				provider: provider || 'opencode',
				model: model || 'big-pickle',
				mode: this.mode,
				user_prompt: userPrompt,
				system_prior: systemPrior
			});
			this.testResult = res;
		} catch (e: unknown) {
			this.error = e instanceof Error ? e.message : 'Failed to run test prompt';
		} finally {
			this.testing = false;
		}
	}
}

export const cavemanState = new CavemanState();
