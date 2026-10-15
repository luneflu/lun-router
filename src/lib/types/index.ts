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
