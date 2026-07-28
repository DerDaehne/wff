import { apiFetch } from './api';

export interface Settings {
	ftp_watts: number | null;
	lthr_bpm: number | null;
	/** Optional: turns climbing speed into a rough power figure (#610). */
	weight_kg: number | null;
	/** Which figure the rider wants to see first (#616); null = default order. */
	primary_metric: string | null;
	/** Optional, only used for the energy estimate (#625). Birth year rather than
	 *  age so it doesn't go stale; sex is 'male' | 'female' because the Keytel
	 *  formula publishes exactly those two coefficient sets. */
	birth_year: number | null;
	sex: string | null;
	/** Shares this rider's relative training-success trend with every other
	 *  rider who has also opted in (#642) — symmetric, null defaults to
	 *  false server-side. */
	compare_opt_in: boolean | null;
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

/** The hardest pulse any ride has recorded, with the ride it came from — an
 *  observation, not a measured maximum (#624). */
export interface ObservedMaxHR {
	bpm: number;
	activity_id: number;
	ridden_at: string;
}

export interface SettingsResponse extends Settings {
	estimates: Estimates;
	gaps: Gap[];
	observed_max_hr: ObservedMaxHR | null;
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
