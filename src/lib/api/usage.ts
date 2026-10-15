import type { UsageFilters, UsageSummary } from '$lib/types';

export async function fetchUsage(days: number, filters: UsageFilters = {}): Promise<UsageSummary> {
	const params = new URLSearchParams({ days: String(days) });
	if (filters.prov) params.set('prov', filters.prov);
	if (filters.key_id) params.set('key_id', filters.key_id);
	if (filters.model) params.set('model', filters.model);

	const res = await fetch(`/api/usage?${params}`);
	if (!res.ok) {
		throw new Error(`Failed to fetch usage: ${res.statusText}`);
	}
	return res.json();
}
