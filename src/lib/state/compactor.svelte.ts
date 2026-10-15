import type { CompactorStatus, CompactorTestResult } from '$lib/types';
import { fetchCompactorStatus, updateCompactor, testCompactorCompress } from '$lib/api/compactor';

export class CompactorState {
	enabled = $state(true);
	engine = $state('embedded-go');
	stats = $state<CompactorStatus['stats']>({
		total_requests: 0,
		compressed_requests: 0,
		bytes_before: 0,
		bytes_after: 0,
		saved_bytes: 0,
		saved_percent: 0,
		recent_hits: []
	});
	supportedFilters = $state<string[]>([
		'json',
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
	testResult = $state<CompactorTestResult | null>(null);

	async load() {
		try {
			this.loading = true;
			this.error = null;
			const data = await fetchCompactorStatus();
			this.enabled = data.enabled;
			this.engine = data.engine ?? 'embedded-go';
			this.stats = data.stats;
			if (data.supported_filters) {
				this.supportedFilters = data.supported_filters;
			}
		} catch (e: unknown) {
			this.error = e instanceof Error ? e.message : 'Failed to load compactor status';
		} finally {
			this.loading = false;
		}
	}

	async toggle() {
		try {
			this.toggling = true;
			this.error = null;
			const data = await updateCompactor(!this.enabled);
			this.enabled = data.enabled;
			this.stats = data.stats;
		} catch (e: unknown) {
			this.error = e instanceof Error ? e.message : 'Failed to update compactor';
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
			const res = await testCompactorCompress(text);
			this.testResult = res;
		} catch (e: unknown) {
			this.error = e instanceof Error ? e.message : 'Failed to test compression';
		} finally {
			this.testing = false;
		}
	}
}

export const compactorState = new CompactorState();
