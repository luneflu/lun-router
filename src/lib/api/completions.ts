import type { ChatCompletionRequest } from '$lib/types';

export interface CompletionCallbacks {
	onDelta?: (delta: string) => void;
}

export async function sendChatCompletion(
	req: ChatCompletionRequest,
	apiKey: string,
	callbacks?: CompletionCallbacks
): Promise<string> {
	const res = await fetch('/v1/chat/completions', {
		method: 'POST',
		headers: {
			'Content-Type': 'application/json',
			Authorization: `Bearer ${apiKey}`
		},
		body: JSON.stringify(req)
	});

	if (!res.ok) {
		const errBody = await res.json().catch(() => ({}));
		throw new Error(errBody.error?.message || `Upstream returned status ${res.status}`);
	}

	if (req.stream) {
		const reader = res.body?.getReader();
		const decoder = new TextDecoder();
		if (!reader) throw new Error('No readable response body');

		let fullText = '';
		let buffer = '';

		while (true) {
			const { done, value } = await reader.read();
			if (done) break;
			buffer += decoder.decode(value, { stream: true });
			const lines = buffer.split('\n');
			buffer = lines.pop() || '';

			for (const line of lines) {
				const trimmed = line.trim();
				if (!trimmed || !trimmed.startsWith('data: ')) continue;
				const dataStr = trimmed.slice(6);
				if (dataStr === '[DONE]') break;
				try {
					const parsed = JSON.parse(dataStr);
					const delta = parsed.choices?.[0]?.delta?.content || '';
					if (delta) {
						fullText += delta;
						callbacks?.onDelta?.(delta);
					}
				} catch {
					// skip non-json
				}
			}
		}
		return fullText;
	}

	const data = await res.json();
	return data.choices?.[0]?.message?.content || '';
}
