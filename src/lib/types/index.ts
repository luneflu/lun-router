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

export interface TokenSaverHit {
	shape: string;
	filter: string;
	saved: number;
}

export interface TokenSaverStats {
	total_requests: number;
	compressed_requests: number;
	bytes_before: number;
	bytes_after: number;
	saved_bytes: number;
	saved_percent: number;
	recent_hits?: TokenSaverHit[];
}

export interface TokenSaverStatus {
	rtk_enabled: boolean;
	engine?: string;
	system_rtk_available?: boolean;
	system_rtk_path?: string;
	stats: TokenSaverStats;
	supported_filters: string[];
}

export interface TokenSaverTestResult {
	filter: string | null;
	original_size: number;
	compressed_size: number;
	saved_bytes: number;
	saved_percent: number;
	output: string;
}
