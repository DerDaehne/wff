import { apiFetch } from './api';
import type { ZoneDistribution, ZoneShares } from './zones';

export interface ActivitySummary {
	id: number;
	started_at: string;
	sport: string;
	elapsed_seconds: number;
	moving_seconds: number;
	distance_meters: number | null;
	training_stress_score: number | null;
	/** The ride's character at a glance in the list (#633) — absent without
	 *  enough recorded pulse, same rule as the ride-detail zones. */
	zones?: ZoneShares | null;
}

export interface Sample {
	time: string;
	lat: number | null;
	lon: number | null;
	altitude_meters: number | null;
	power_watts: number | null;
	heart_rate: number | null;
}

export async function listActivities(): Promise<ActivitySummary[]> {
	const res = await apiFetch('/api/activities');
	const data = await res.json();
	return data ?? [];
}

export async function getActivitySamples(id: number): Promise<Sample[]> {
	const res = await apiFetch(`/api/activities/${id}/samples`);
	const data = await res.json();
	return data ?? [];
}

export interface WeatherSummary {
	avg_wind_speed_mps: number | null;
	avg_headwind_mps: number | null;
	avg_temperature_celsius: number | null;
	buckets_enriched: number;
}

export async function getActivityWeather(id: number): Promise<WeatherSummary> {
	const res = await apiFetch(`/api/activities/${id}/weather`);
	return res.json();
}

/** One plain-language statement about a ride, with the number behind it as a
 *  secondary label. Wording and thresholds come from the backend
 *  (internal/analyze/ridestory.go) so they have a single home — see #601. */
export interface RideStatement {
	text: string;
	metric?: string;
	kind:
		| 'effort'
		| 'load'
		| 'pace'
		| 'climb'
		| 'endurance'
		| 'context'
		| 'comparison'
		// Dashboard kinds (#602): current form, and whether fitness is going up.
		| 'form'
		| 'trend'
		// Whether the aerobic base itself is growing, across comparable rides
		// only (#619) — a different question from 'trend' over all rides.
		| 'endurance_trend'
		// How the effort was spread across the heart-rate bands (#621).
		| 'zones'
		// What the ride cost in energy, estimated from the pulse (#625).
		| 'calories'
		// A personal record this ride just set — own history only, never a
		// comparison with other riders (#636).
		| 'milestone'
		| 'hint_profile'
		| 'hint_history';
}

/** A headline figure, split so the number can be set large and the unit small. */
export interface RideStat {
	value: string;
	unit: string;
	label: string;
}

/** A percentage meant to be drawn as a filled bar. `percent` is clamped to
 *  0–100 for the bar; `label` carries the real, unclamped reading. */
export interface RideGauge {
	percent: number;
	label: string;
	caption: string;
}

export interface RideStory {
	title: string;
	subtitle: string;
	stats: RideStat[];
	/** The one bar a view shows: ride intensity, or training level (#611). */
	gauge?: RideGauge;
	statements: RideStatement[];
	/** Time per heart-rate band (#621). Absent without a threshold heart rate
	 *  or with too little pulse recorded. */
	zones?: ZoneDistribution;
}

export async function getActivityStory(id: number): Promise<RideStory> {
	const res = await apiFetch(`/api/activities/${id}/story`);
	return res.json();
}

export function formatDistance(meters: number | null): string {
	if (meters === null) return '–';
	return `${(meters / 1000).toFixed(1)} km`;
}

export function formatDuration(seconds: number): string {
	const h = Math.floor(seconds / 3600);
	const m = Math.floor((seconds % 3600) / 60);
	return h > 0 ? `${h}:${String(m).padStart(2, '0')} h` : `${m} min`;
}
