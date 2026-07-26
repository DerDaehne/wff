import { apiFetch } from './api';

export interface Settings {
	ftp_watts: number | null;
	lthr_bpm: number | null;
}

/** A threshold derived from the rider's own rides, carrying the ride it came
 *  from so the number is traceable rather than magic (#609). */
export interface Estimate {
	value: number;
	best_20min: number;
	activity_id: number;
	ridden_at: string;
}

export interface Estimates {
	ftp_watts: Estimate | null;
	lthr_bpm: Estimate | null;
}

export interface SettingsResponse extends Settings {
	estimates: Estimates;
}

export async function getSettings(): Promise<SettingsResponse> {
	const res = await apiFetch('/api/me/settings');
	const data = await res.json();
	return { ...data, estimates: data.estimates ?? { ftp_watts: null, lthr_bpm: null } };
}

export async function updateSettings(patch: Partial<Settings>): Promise<Settings> {
	const res = await apiFetch('/api/me/settings', {
		method: 'PATCH',
		headers: { 'content-type': 'application/json' },
		body: JSON.stringify(patch)
	});
	return res.json();
}
