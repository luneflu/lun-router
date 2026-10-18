export interface APIKey {
	id: string;
	key: string;
	name: string;
	machineId?: string;
	isActive: boolean;
	createdAt: string;
}

export interface CreateKeyPayload {
	name: string;
	key?: string;
}

export interface ChatMessage {
	role: 'user' | 'assistant' | 'system';
	content: string;
}

export interface ChatCompletionRequest {
	model: string;
	messages: ChatMessage[];
	stream?: boolean;
}

export interface ModelCapabilities {
	vision?: boolean;
	tools?: boolean;
	reasoning?: boolean;
	thinking_format?: string;
	thinking_levels?: string[];
	thinking_can_disable?: boolean;
	context_window?: number;
	max_output?: number;
}

export interface ProviderModel {
	id: string;
	object?: string;
	owned_by?: string;
	enabled: boolean;
	capabilities?: ModelCapabilities;
	thinking_levels?: string[];
	context_length?: number;
	max_completion_tokens?: number;
}

export interface AntigravityAccount {
	id: string;
	email: string;
	projectId: string;
	accessToken?: string;
	refreshToken?: string;
	expiresAt?: string;
	isActive: boolean;
	createdAt: string;
}

export interface KilocodeAccount {
	id: string;
	email: string;
	orgId?: string;
	accessToken?: string;
	isActive: boolean;
	createdAt: string;
}

export interface AntigravityQuotaItem {
	name: string;
	modelKey?: string;
	used: number;
	total: number;
	remainingPercentage: number;
	resetAt?: string | null;
	unlimited?: boolean;
}

export interface AntigravityAccountUsage {
	accountId: string;
	email: string;
	plan: string;
	message?: string;
	quotas: Record<string, AntigravityQuotaItem>;
	fetchedAt: string;
}

export interface CompactorHit {
	shape: string;
	filter: string;
	saved: number;
}

export interface CompactorStats {
	total_requests: number;
	compressed_requests: number;
	bytes_before: number;
	bytes_after: number;
	saved_bytes: number;
	saved_percent: number;
	recent_hits?: CompactorHit[];
}

export interface CompactorStatus {
	enabled: boolean;
	engine?: string;
	stats: CompactorStats;
	supported_filters: string[];
}

export interface CompactorTestResult {
	filter: string | null;
	original_size: number;
	compressed_size: number;
	saved_bytes: number;
	saved_percent: number;
	output: string;
}

export interface UsageTotals {
	requests: number;
	errors: number;
	in: number;
	out: number;
	cached: number;
	reasoning: number;
	saved_bytes: number;
	avg_ms: number;
}

export interface UsageDay {
	date: string;
	requests: number;
	errors: number;
	in: number;
	out: number;
}

export interface UsageKeyRow {
	key_id: string;
	key_name: string;
	requests: number;
	errors: number;
	in: number;
	out: number;
	last_ts: string;
}

export interface UsageModelRow {
	prov: string;
	model: string;
	requests: number;
	errors: number;
	in: number;
	out: number;
}

export interface UsageEventRow {
	ts: string;
	key_id?: string;
	key_name?: string;
	prov: string;
	acct?: string;
	model: string;
	req_model?: string;
	ep?: string;
	stream?: boolean;
	status: number;
	ms: number;
	up_ms?: number;
	in: number;
	out: number;
	cached?: number;
	reasoning?: number;
	tok_src?: string;
	err?: string;
	saved?: number;
	filters?: string[];
	agent?: string;
	sess?: string;
}

export interface UsageProviderRow {
	prov: string;
	requests: number;
	errors: number;
	in: number;
	out: number;
}

export interface UsageHourRow {
	hour: number;
	requests: number;
	errors: number;
	in: number;
	out: number;
}

export interface UsageFilters {
	prov?: string;
	key_id?: string;
	model?: string;
}

export interface UsageSummary {
	days: number;
	from: string;
	to: string;
	filters: UsageFilters;
	totals: UsageTotals;
	by_day: UsageDay[];
	by_hour?: UsageHourRow[];
	by_provider: UsageProviderRow[];
	by_key: UsageKeyRow[];
	by_model: UsageModelRow[];
	recent: UsageEventRow[];
}

export interface CavemanModeInfo {
	id: string;
	name: string;
	description: string;
	tokens_est: number;
}

export interface CavemanStats {
	total_requests: number;
	injected_requests: number;
	estimated_saved_pct: number;
}

export interface CavemanStatus {
	enabled: boolean;
	mode: string;
	active_prompt: string;
	available_modes: CavemanModeInfo[];
	custom_prompt?: string;
	stats: CavemanStats;
}

export interface CavemanTestResult {
	applied: boolean;
	provider?: string;
	model?: string;
	mode: string;
	injected_prompt: string;
	messages: Array<{ role: string; content: string }>;
	prompt_tokens_est: number;
	estimated_savings: string;
	timestamp: string;
}

export interface PonytailModeInfo {
	id: string;
	name: string;
	description: string;
}

export interface PonytailStats {
	total_requests: number;
	injected_requests: number;
}

export interface PonytailStatus {
	enabled: boolean;
	mode: string;
	active_prompt: string;
	available_modes: PonytailModeInfo[];
	custom_prompt?: string;
	stats: PonytailStats;
}

export interface PonytailTestResult {
	applied: boolean;
	provider?: string;
	model?: string;
	mode: string;
	injected_prompt: string;
	messages: Array<{ role: string; content: string }>;
	timestamp: string;
}

export interface AuthStatus {
	auth_enabled: boolean;
	setup_required: boolean;
	authenticated: boolean;
}
