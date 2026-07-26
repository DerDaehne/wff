import { apiFetch } from './api';

export interface Settings {
	ftp_watts: number | null;
	lthr_bpm: number | null;
}

export async function getSettings(): Promise<Settings> {
	const res = await apiFetch('/api/me/settings');
	return res.json();
}

export async function updateSettings(patch: Partial<Settings>): Promise<Settings> {
	const res = await apiFetch('/api/me/settings', {
		method: 'PATCH',
		headers: { 'content-type': 'application/json' },
		body: JSON.stringify(patch)
	});
	return res.json();
}
