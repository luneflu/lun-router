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
