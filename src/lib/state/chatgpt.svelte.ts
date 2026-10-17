import {
	fetchChatGPTStatus,
	updateChatGPTStatus,
	installChatGPTBrowser,
	uploadChatGPTCookies,
	deleteChatGPTCookies,
	updateChatGPTTools,
	type ChatGPTStatus
} from '$lib/api/providers';

export class ChatGPTState {
	enabled = $state(false);
	installed = $state(false);
	browserPath = $state<string | undefined>(undefined);
	browserRunning = $state(false);
	browserPid = $state<number | undefined>(undefined);
	browserProcs = $state(0);
	browserRssMb = $state(0);
	installState = $state<string>('idle');
	installProgress = $state<number>(0);
	installError = $state<string | undefined>(undefined);
	hasCookies = $state(false);
	uploadingCookies = $state(false);
	shellEnabled = $state(false);
	passthroughTools = $state(true);
	workDir = $state<string>('');
	loading = $state(false);
	toggling = $state(false);
	installing = $state(false);
	error = $state<string | null>(null);
	private pollInterval: ReturnType<typeof setInterval> | null = null;
	private ramInterval: ReturnType<typeof setInterval> | null = null;

	startRamPolling() {
		if (this.ramInterval) return;
		this.ramInterval = setInterval(async () => {
			try {
				const data = await fetchChatGPTStatus();
				this.browserRunning = data.browser_running ?? false;
				this.browserPid = data.browser_pid;
				this.browserProcs = data.browser_procs ?? 0;
				this.browserRssMb = data.browser_rss_mb ?? 0;
				this.hasCookies = data.has_cookies ?? false;
			} catch (_) {}
		}, 2000);
	}

	stopRamPolling() {
		if (this.ramInterval) {
			clearInterval(this.ramInterval);
			this.ramInterval = null;
		}
	}

	async load() {
		try {
			this.loading = true;
			this.error = null;
			const data = await fetchChatGPTStatus();
			this.enabled = data.enabled;
			this.installed = data.installed;
			this.hasCookies = data.has_cookies ?? false;
			this.shellEnabled = data.shell_enabled ?? false;
			this.passthroughTools = data.passthrough_tools ?? true;
			this.workDir = data.work_dir ?? '';
			this.browserPath = data.browser_path;
			this.browserRunning = data.browser_running ?? false;
			this.browserPid = data.browser_pid;
			this.browserProcs = data.browser_procs ?? 0;
			this.browserRssMb = data.browser_rss_mb ?? 0;
			this.installState = data.install_state ?? 'idle';
			this.installProgress = data.install_progress ?? 0;
			this.installError = data.install_error;

			if (this.installState === 'downloading' || this.installState === 'extracting') {
				this.startPolling();
			}
		} catch (e: unknown) {
			this.error = e instanceof Error ? e.message : 'Failed to load ChatGPT status';
		} finally {
			this.loading = false;
		}
	}

	async toggle(targetEnabled: boolean): Promise<boolean> {
		try {
			this.toggling = true;
			this.error = null;
			const res = await updateChatGPTStatus(targetEnabled);
			this.enabled = res.enabled;
			return true;
		} catch (e: unknown) {
			this.error = e instanceof Error ? e.message : 'Failed to update ChatGPT status';
			return false;
		} finally {
			this.toggling = false;
		}
	}

	async startInstall() {
		try {
			this.installing = true;
			this.installState = 'downloading';
			this.installProgress = 0;
			this.installError = undefined;
			this.error = null;

			await installChatGPTBrowser();
			this.startPolling();
		} catch (e: unknown) {
			this.installing = false;
			this.installState = 'error';
			this.error = e instanceof Error ? e.message : 'Failed to start browser installation';
		}
	}

	async uploadCookies(jsonStr: string): Promise<boolean> {
		try {
			this.uploadingCookies = true;
			this.error = null;
			const res = await uploadChatGPTCookies(jsonStr);
			this.hasCookies = res.has_cookies;
			return true;
		} catch (e: unknown) {
			this.error = e instanceof Error ? e.message : 'Failed to upload cookies';
			return false;
		} finally {
			this.uploadingCookies = false;
		}
	}

	async deleteCookies(): Promise<boolean> {
		try {
			this.uploadingCookies = true;
			this.error = null;
			const res = await deleteChatGPTCookies();
			this.hasCookies = res.has_cookies;
			return true;
		} catch (e: unknown) {
			this.error = e instanceof Error ? e.message : 'Failed to delete cookies';
			return false;
		} finally {
			this.uploadingCookies = false;
		}
	}

	async setShellEnabled(enabled: boolean): Promise<boolean> {
		try {
			this.error = null;
			const res = await updateChatGPTTools({ shell_enabled: enabled });
			this.shellEnabled = res.shell_enabled;
			this.workDir = res.work_dir;
			return true;
		} catch (e: unknown) {
			this.error = e instanceof Error ? e.message : 'Failed to update tool settings';
			return false;
		}
	}

	async setPassthroughTools(enabled: boolean): Promise<boolean> {
		try {
			this.error = null;
			const res = await updateChatGPTTools({ passthrough_tools: enabled });
			this.passthroughTools = res.passthrough_tools;
			return true;
		} catch (e: unknown) {
			this.error = e instanceof Error ? e.message : 'Failed to update tool routing';
			return false;
		}
	}

	async setWorkDir(dir: string): Promise<boolean> {
		try {
			this.error = null;
			const res = await updateChatGPTTools({ work_dir: dir });
			this.workDir = res.work_dir;
			return true;
		} catch (e: unknown) {
			this.error = e instanceof Error ? e.message : 'Failed to update work directory';
			return false;
		}
	}

	private startPolling() {
		if (this.pollInterval) return;
		this.installing = true;

		this.pollInterval = setInterval(async () => {
			try {
				const data = await fetchChatGPTStatus();
				this.installState = data.install_state ?? 'idle';
				this.installProgress = data.install_progress ?? 0;
				this.installError = data.install_error;
				this.installed = data.installed;

				if (this.installState === 'ready') {
					this.stopPolling();
					this.installed = true;
					this.installing = false;
					// Automatically enable after successful install!
					await this.toggle(true);
				} else if (this.installState === 'error') {
					this.stopPolling();
					this.installing = false;
					this.error = this.installError || 'Installation failed';
				}
			} catch (e) {
				// keep polling
			}
		}, 1000);
	}

	private stopPolling() {
		if (this.pollInterval) {
			clearInterval(this.pollInterval);
			this.pollInterval = null;
		}
	}
}

export const chatgptState = new ChatGPTState();
