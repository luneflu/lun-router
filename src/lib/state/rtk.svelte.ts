import type { RTKStats, RTKTestResult } from '$lib/types';
import { fetchRTKStatus, updateRTKConfig, testRTKCompress } from '$lib/api/rtk';

export class RTKState {
	rtkEnabled = $state(true);
	engine = $state('embedded-go');
	systemRtkAvailable = $state(false);
	systemRtkPath = $state<string | undefined>(undefined);
	stats = $state<RTKStats>({
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
		'cargo-test',
		'pytest',
		'go-test',
		'dedup-log',
		'smart-truncate'
	]);
	loading = $state(false);
	toggling = $state(false);
	testing = $state(false);
	error = $state<string | null>(null);
	testResult = $state<RTKTestResult | null>(null);

	async load() {
		try {
			this.loading = true;
			this.error = null;
			const data = await fetchRTKStatus();
			this.rtkEnabled = data.rtk_enabled;
			this.engine = data.engine ?? 'embedded-go';
			this.systemRtkAvailable = !!data.system_rtk_available;
			this.systemRtkPath = data.system_rtk_path;
			this.stats = data.stats;
			if (data.supported_filters) {
				this.supportedFilters = data.supported_filters;
			}
		} catch (e: unknown) {
			this.error = e instanceof Error ? e.message : 'Failed to load RTK status';
		} finally {
			this.loading = false;
		}
	}

	async toggleRTK() {
		try {
			this.toggling = true;
			this.error = null;
			const target = !this.rtkEnabled;
			const data = await updateRTKConfig(target);
			this.rtkEnabled = data.rtk_enabled;
			this.stats = data.stats;
		} catch (e: unknown) {
			this.error = e instanceof Error ? e.message : 'Failed to update RTK';
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
			const res = await testRTKCompress(text);
			this.testResult = res;
		} catch (e: unknown) {
			this.error = e instanceof Error ? e.message : 'Failed to test compression';
		} finally {
			this.testing = false;
		}
	}
}

export const rtkState = new RTKState();

// Aliases for backward compatibility
export const TokenSaverState = RTKState;
export const tokenSaverState = rtkState;
