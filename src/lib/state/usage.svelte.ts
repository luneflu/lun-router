import type { UsageKeyRow, UsageModelRow, UsageSummary } from '$lib/types';
import { fetchUsage } from '$lib/api/usage';

const RANGE_OPTIONS = [1, 7, 30, 90];

export class UsageState {
	days = $state(7);
	prov = $state('');
	keyId = $state('');
	model = $state('');
	summary = $state<UsageSummary | null>(null);
	loading = $state(false);
	error = $state<string | null>(null);

	/** Options for the filter selects, taken from the unfiltered window so that
	 * picking a filter does not shrink the list you are picking from. */
	keyOptions = $state<UsageKeyRow[]>([]);
	modelOptions = $state<UsageModelRow[]>([]);
	readonly ranges = RANGE_OPTIONS;

	get filtered(): boolean {
		return this.prov !== '' || this.keyId !== '' || this.model !== '';
	}

	async load(days = this.days) {
		this.days = days;
		await this.fetch({ reloadOptions: true });
	}

	async applyFilters() {
		await this.fetch({ reloadOptions: false });
	}

	async resetFilters() {
		this.prov = '';
		this.keyId = '';
		this.model = '';
		await this.fetch({ reloadOptions: true });
	}

	private async fetch({ reloadOptions }: { reloadOptions: boolean }) {
		try {
			this.loading = true;
			this.error = null;
			const summary = await fetchUsage(this.days, {
				prov: this.prov || undefined,
				key_id: this.keyId || undefined,
				model: this.model || undefined
			});
			this.summary = summary;

			if (reloadOptions || this.keyOptions.length === 0) {
				const options = await fetchUsage(this.days);
				this.keyOptions = options.by_key;
				this.modelOptions = options.by_model;
			}
		} catch (e: unknown) {
			this.error = e instanceof Error ? e.message : 'Failed to load usage';
		} finally {
			this.loading = false;
		}
	}
}

export const usageState = new UsageState();
