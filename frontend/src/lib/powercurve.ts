import { apiFetch } from './api';

/** One ride's best average power at a fixed duration (#593) — the same
 *  figure the ride's own power curve stored, read back across rides to see
 *  whether it's trending up. Natural day-to-day variance included on
 *  purpose: this is not a monotonic best-ever ratchet. */
export interface PowerCurveHistoryPoint {
	activity_id: number;
	started_at: string;
	watts: number;
}

export async function getPowerCurve(durationSeconds: number): Promise<PowerCurveHistoryPoint[]> {
	const res = await apiFetch(`/api/power-curve?duration=${durationSeconds}`);
	const data = await res.json();
	return data ?? [];
}
