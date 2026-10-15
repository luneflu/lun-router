import type { ChatCompletionRequest } from '$lib/types';

export interface CompletionCallbacks {
	onDelta?: (delta: string) => void;
	signal?: AbortSignal;
	headers?: Record<string, string>;
}

export async function sendChatCompletion(
	req: ChatCompletionRequest,
	apiKey?: string,
	callbacks?: CompletionCallbacks
): Promise<string> {
	const headers: Record<string, string> = {
		'Content-Type': 'application/json',
		...(callbacks?.headers || {})
	};
	if (apiKey) {
		headers['Authorization'] = `Bearer ${apiKey}`;
	}

	const res = await fetch('/api/chat/completions', {
		method: 'POST',
		headers,
		body: JSON.stringify(req),
		signal: callbacks?.signal
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
					let delta = '';
					if (typeof parsed.choices?.[0]?.delta?.content === 'string') {
						delta = parsed.choices[0].delta.content;
					} else if (typeof parsed.delta === 'string') {
						delta = parsed.delta;
					} else if (typeof parsed.delta?.text === 'string') {
						delta = parsed.delta.text;
					} else if (typeof parsed.text === 'string') {
						delta = parsed.text;
					} else if (typeof parsed.part?.text === 'string') {
						delta = parsed.part.text;
					}

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
	let content = data.choices?.[0]?.message?.content;
	if (typeof content !== 'string') {
		content = data.output?.[0]?.content?.[0]?.text || data.text || '';
	}
	return content;
}
