import { apiFetch } from './api';
import type { RideStatement } from './rides';

/** One calendar week of riding, aggregated. Weeks rather than rides because a
 *  single ride says nothing about progress — one tailwind evening beats a good
 *  week of training on average speed (#618). */
export interface Week {
	start: string;
	rides: number;
	distance_meters: number;
	moving_seconds: number;
	elevation_gain_meters: number;
	avg_speed_kmh: number;
}

export interface Progress {
	weeks: Week[];
	statements: RideStatement[];
}

export async function getProgress(): Promise<Progress> {
	const res = await apiFetch('/api/progress');
	const data = await res.json();
	return { weeks: data.weeks ?? [], statements: data.statements ?? [] };
}

/** The figures the weekly chart can show. Kept here rather than in the page so
 *  #616 (rider picks a preferred metric) has one list to point at. */
export const progressMetrics = [
	{
		key: 'avg_speed_kmh' as const,
		label: 'Tempo',
		color: 'var(--chart-speed)',
		format: (v: number) => `${v.toFixed(1)} km/h`
	},
	{
		key: 'distance_meters' as const,
		label: 'Kilometer',
		color: 'var(--chart-ctl)',
		format: (v: number) => `${Math.round(v / 1000)} km`
	},
	{
		key: 'moving_seconds' as const,
		label: 'Fahrzeit',
		color: 'var(--chart-power)',
		format: (v: number) => `${(v / 3600).toFixed(1)} h`
	},
	{
		key: 'elevation_gain_meters' as const,
		label: 'Höhenmeter',
		color: 'var(--chart-elevation)',
		format: (v: number) => `${Math.round(v)} hm`
	}
];

export type ProgressMetricKey = (typeof progressMetrics)[number]['key'];
