import { apiFetch } from './api';

/** One ride singled out from a year, with the number that made it stand out
 *  — TSS for the hardest ride, metres for the longest (#638). */
export interface RideHighlight {
	activity_id: number;
	started_at: string;
	value: number;
}

/** A calendar year's rides summed up, plus its two standout rides. Pure
 *  summary of stored figures — nothing here is a newly derived metric. */
export interface YearReview {
	year: number;
	ride_count: number;
	distance_meters: number;
	elevation_gain_meters: number;
	moving_seconds: number;
	hardest_ride?: RideHighlight | null;
	longest_ride?: RideHighlight | null;
}

export async function getYearReview(year: number): Promise<YearReview> {
	const res = await apiFetch(`/api/me/year-review?year=${year}`);
	return res.json();
}
