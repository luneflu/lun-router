export interface ToastItem {
	id: string;
	type: 'success' | 'error' | 'info';
	title: string;
	description?: string;
}

export class ToastManager {
	items = $state<ToastItem[]>([]);

	show(item: Omit<ToastItem, 'id'>, durationMs = 3500) {
		const id = Math.random().toString(36).slice(2);
		this.items = [...this.items, { ...item, id }];
		setTimeout(() => {
			this.dismiss(id);
		}, durationMs);
	}

	success(title: string, description?: string) {
		this.show({ type: 'success', title, description });
	}

	error(title: string, description?: string) {
		this.show({ type: 'error', title, description }, 4500);
	}

	info(title: string, description?: string) {
		this.show({ type: 'info', title, description });
	}

	dismiss(id: string) {
		this.items = this.items.filter((t) => t.id !== id);
	}
}

export const toast = new ToastManager();
