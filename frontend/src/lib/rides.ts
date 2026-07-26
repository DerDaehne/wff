import { apiFetch } from './api';

export interface ActivitySummary {
	id: number;
	started_at: string;
	sport: string;
	elapsed_seconds: number;
	moving_seconds: number;
	distance_meters: number | null;
	training_stress_score: number | null;
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
	kind: 'effort' | 'load' | 'context' | 'comparison' | 'hint_profile' | 'hint_history';
}

export interface RideStory {
	headline: string;
	statements: RideStatement[];
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
