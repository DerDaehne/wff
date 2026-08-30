import { apiFetch } from './api';

/** One of a rider's bikes, with its computed odometer and chain status
 *  (#637). distance_km and chain_due_km are derived on every read, not a
 *  running counter that could drift out of sync with the rides it
 *  describes. */
export interface Bike {
	id: number;
	name: string;
	active: boolean;
	retired_at: string | null;
	distance_km: number;
	chain_interval_km: number;
	/** Km remaining before the chain reminder fires — negative once the
	 *  interval has been exceeded. */
	chain_due_km: number;
	/** Cumulative figures for the per-bike comparison view (#731) — the same
	 *  honest, Strava/Garmin-Gear-style totals rather than a constructed
	 *  "which bike is faster" score (speed is too route/wind-dependent to
	 *  attribute fairly to the bike). */
	ride_count: number;
	moving_seconds: number;
	elevation_gain_meters: number;
	/** 0 (not null) when there's no moving time yet — nothing to divide by. */
	avg_speed_kmh: number;
}

export async function listBikes(): Promise<Bike[]> {
	const res = await apiFetch('/api/bikes');
	const data = await res.json();
	return data ?? [];
}

export async function createBike(name: string): Promise<Bike[]> {
	const res = await apiFetch('/api/bikes', {
		method: 'POST',
		headers: { 'content-type': 'application/json' },
		body: JSON.stringify({ name })
	});
	return res.json();
}

export async function updateBike(
	id: number,
	patch: { name?: string; chain_interval_km?: number; retired?: boolean }
): Promise<Bike[]> {
	const res = await apiFetch(`/api/bikes/${id}`, {
		method: 'PATCH',
		headers: { 'content-type': 'application/json' },
		body: JSON.stringify(patch)
	});
	return res.json();
}

export async function activateBike(id: number): Promise<Bike[]> {
	const res = await apiFetch(`/api/bikes/${id}/activate`, { method: 'POST' });
	return res.json();
}

export async function markChainReplaced(id: number): Promise<Bike[]> {
	const res = await apiFetch(`/api/bikes/${id}/chain-replaced`, { method: 'POST' });
	return res.json();
}
