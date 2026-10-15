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

export interface ProviderModel {
	id: string;
	object?: string;
	owned_by?: string;
	enabled: boolean;
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

export interface RTKHit {
	shape: string;
	filter: string;
	saved: number;
}

export interface RTKStats {
	total_requests: number;
	compressed_requests: number;
	bytes_before: number;
	bytes_after: number;
	saved_bytes: number;
	saved_percent: number;
	recent_hits?: RTKHit[];
}

export interface RTKStatus {
	rtk_enabled: boolean;
	engine?: string;
	system_rtk_available?: boolean;
	system_rtk_path?: string;
	stats: RTKStats;
	supported_filters: string[];
}

export interface RTKTestResult {
	filter: string | null;
	original_size: number;
	compressed_size: number;
	saved_bytes: number;
	saved_percent: number;
	output: string;
}

export type TokenSaverHit = RTKHit;
export type TokenSaverStats = RTKStats;
export type TokenSaverStatus = RTKStatus;
export type TokenSaverTestResult = RTKTestResult;

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
	auto_update: boolean;
	last_sync: string | null;
	upstream_commit: string;
	upstream_commit_msg: string;
	upstream_commit_date: string;
	upstream_url: string;
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
	auto_update: boolean;
	last_sync: string | null;
	upstream_commit: string;
	upstream_commit_msg: string;
	upstream_commit_date: string;
	upstream_url: string;
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
