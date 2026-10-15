import type { TokenSaverStats, TokenSaverTestResult } from '$lib/types';
import { fetchTokenSaverStatus, updateTokenSaver, testTokenSaver } from '$lib/api/tokensaver';

export class TokenSaverState {
	rtkEnabled = $state(true);
	engine = $state('embedded-go');
	systemRtkAvailable = $state(false);
	systemRtkPath = $state<string | undefined>(undefined);
	stats = $state<TokenSaverStats>({
		total_requests: 0,
		compressed_requests: 0,
		bytes_before: 0,
		bytes_after: 0,
		saved_bytes: 0,
		saved_percent: 0,
		recent_hits: []
	});
	supportedFilters = $state<string[]>([
		'git-diff',
		'git-status',
		'git-log',
		'grep',
		'find',
		'ls',
		'tree',
		'build-output',
		'search-list',
		'read-numbered',
		'dedup-log',
		'smart-truncate'
	]);
	loading = $state(false);
	toggling = $state(false);
	testing = $state(false);
	error = $state<string | null>(null);
	testResult = $state<TokenSaverTestResult | null>(null);

	async load() {
		try {
			this.loading = true;
			this.error = null;
			const data = await fetchTokenSaverStatus();
			this.rtkEnabled = data.rtk_enabled;
			this.engine = data.engine ?? 'embedded-go';
			this.systemRtkAvailable = !!data.system_rtk_available;
			this.systemRtkPath = data.system_rtk_path;
			this.stats = data.stats;
			if (data.supported_filters) {
				this.supportedFilters = data.supported_filters;
			}
		} catch (e: unknown) {
			this.error = e instanceof Error ? e.message : 'Failed to load Token Saver status';
		} finally {
			this.loading = false;
		}
	}

	async toggleRTK() {
		try {
			this.toggling = true;
			this.error = null;
			const target = !this.rtkEnabled;
			const data = await updateTokenSaver(target);
			this.rtkEnabled = data.rtk_enabled;
			this.stats = data.stats;
		} catch (e: unknown) {
			this.error = e instanceof Error ? e.message : 'Failed to update Token Saver';
		} finally {
			this.toggling = false;
		}
	}

	async runTest(text: string) {
		if (!text.trim()) {
			this.testResult = null;
			return;
		}
		try {
			this.testing = true;
			this.error = null;
			const res = await testTokenSaver(text);
			this.testResult = res;
		} catch (e: unknown) {
			this.error = e instanceof Error ? e.message : 'Failed to test compression';
		} finally {
			this.testing = false;
		}
	}
}

export const tokenSaverState = new TokenSaverState();
