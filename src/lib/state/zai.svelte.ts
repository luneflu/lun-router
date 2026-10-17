import {
	fetchZaiStatus,
	updateZaiStatus,
	installZaiBrowser,
	updateZaiTools,
	type ZaiStatus
} from '$lib/api/providers';

/**
 * ZaiState drives the z.ai provider page.
 *
 * The provider itself is pure HTTP; a browser exists only to mint Aliyun captcha
 * tokens, which are single-use and cannot be computed without running Aliyun's
 * device-signing SDK.
 */
export class ZaiState {
	enabled = $state(false);
	installed = $state(false);
	browserPath = $state<string | undefined>(undefined);
	browserRunning = $state(false);
	installState = $state<string>('idle');
	installProgress = $state<number>(0);
	installError = $state<string | undefined>(undefined);
	passthroughTools = $state(true);
	guestMode = $state(true);
	loading = $state(false);
	toggling = $state(false);
	installing = $state(false);
	error = $state<string | null>(null);

	private pollInterval: ReturnType<typeof setInterval> | null = null;
	private statusInterval: ReturnType<typeof setInterval> | null = null;

	#apply(data: ZaiStatus) {
		this.enabled = data.enabled;
		this.installed = data.installed ?? false;
		this.browserPath = data.browser_path;
		this.browserRunning = data.browser_running ?? false;
		this.installState = data.install_state ?? 'idle';
		this.installProgress = data.install_progress ?? 0;
		this.installError = data.install_error;
		this.passthroughTools = data.passthrough_tools ?? true;
		this.guestMode = data.guest_mode ?? true;
	}

	startStatusPolling() {
		if (this.statusInterval) return;
		this.statusInterval = setInterval(async () => {
			try {
				this.#apply(await fetchZaiStatus());
			} catch (_) {
				/* transient; keep the last known state */
			}
		}, 3000);
	}

	stopStatusPolling() {
		if (this.statusInterval) {
			clearInterval(this.statusInterval);
			this.statusInterval = null;
		}
	}

	async load() {
		try {
			this.loading = true;
			this.error = null;
			this.#apply(await fetchZaiStatus());
			if (this.installState === 'downloading' || this.installState === 'extracting') {
				this.startInstallPolling();
			}
		} catch (e: unknown) {
			this.error = e instanceof Error ? e.message : 'Failed to load z.ai status';
		} finally {
			this.loading = false;
		}
	}

	async toggle(targetEnabled: boolean): Promise<boolean> {
		try {
			this.toggling = true;
			this.error = null;
			const res = await updateZaiStatus(targetEnabled);
			this.enabled = res.enabled;
			if (this.enabled) {
				this.startStatusPolling();
			} else {
				this.stopStatusPolling();
			}
			return true;
		} catch (e: unknown) {
			this.error = e instanceof Error ? e.message : 'Failed to update z.ai status';
			return false;
		} finally {
			this.toggling = false;
		}
	}

	async installBrowser(): Promise<boolean> {
		try {
			this.installing = true;
			this.error = null;
			await installZaiBrowser();
			this.startInstallPolling();
			return true;
		} catch (e: unknown) {
			this.error = e instanceof Error ? e.message : 'Failed to start browser install';
			return false;
		} finally {
			this.installing = false;
		}
	}

	startInstallPolling() {
		if (this.pollInterval) return;
		this.pollInterval = setInterval(async () => {
			try {
				this.#apply(await fetchZaiStatus());
				if (this.installState !== 'downloading' && this.installState !== 'extracting') {
					if (this.pollInterval) {
						clearInterval(this.pollInterval);
						this.pollInterval = null;
					}
				}
			} catch (_) {
				/* keep polling until the state settles */
			}
		}, 1500);
	}

	async setPassthroughTools(value: boolean): Promise<boolean> {
		try {
			this.error = null;
			const res = await updateZaiTools({ passthrough_tools: value });
			this.passthroughTools = res.passthrough_tools;
			return true;
		} catch (e: unknown) {
			this.error = e instanceof Error ? e.message : 'Failed to update tool routing';
			return false;
		}
	}

	destroy() {
		this.stopStatusPolling();
		if (this.pollInterval) {
			clearInterval(this.pollInterval);
			this.pollInterval = null;
		}
	}
}

export const zaiState = new ZaiState();
