import { apiFetch } from './api';

export interface Settings {
	ftp_watts: number | null;
	lthr_bpm: number | null;
	/** Optional: turns climbing speed into a rough power figure (#610). */
	weight_kg: number | null;
	/** Which figure the rider wants to see first (#616); null = default order. */
	primary_metric: string | null;
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

/** Something the app can't tell you yet, plus the ride that would change it.
 *  The instruction is the point — "you're missing an FTP value" is a dead end
 *  for someone who doesn't know what FTP is (#612). */
export interface Gap {
	key: string;
	unlocks: string;
	instruction: string;
}

export interface SettingsResponse extends Settings {
	estimates: Estimates;
	gaps: Gap[];
}

export async function getSettings(): Promise<SettingsResponse> {
	const res = await apiFetch('/api/me/settings');
	const data = await res.json();
	return {
		...data,
		estimates: data.estimates ?? { ftp_watts: null, lthr_bpm: null },
		gaps: data.gaps ?? []
	};
}

export async function updateSettings(patch: Partial<Settings>): Promise<Settings> {
	const res = await apiFetch('/api/me/settings', {
		method: 'PATCH',
		headers: { 'content-type': 'application/json' },
		body: JSON.stringify(patch)
	});
	return res.json();
}
