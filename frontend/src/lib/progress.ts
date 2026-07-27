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

/** One week's efficiency: how much speed (or power) came out per 100
 *  heartbeats, averaged over the rides that were comparable with each other
 *  (#619). Weeks without a comparable ride are simply absent. */
export interface EnduranceWeek {
	start: string;
	rides: number;
	value: number;
}

export interface EnduranceTrend {
	weeks: EnduranceWeek[];
	from_power: boolean;
	unit: string;
	statements: RideStatement[];
}

export interface Progress {
	weeks: Week[];
	statements: RideStatement[];
	endurance: EnduranceTrend;
}

export async function getProgress(): Promise<Progress> {
	const res = await apiFetch('/api/progress');
	const data = await res.json();
	return {
		weeks: data.weeks ?? [],
		statements: data.statements ?? [],
		endurance: {
			weeks: data.endurance?.weeks ?? [],
			from_power: data.endurance?.from_power ?? false,
			unit: data.endurance?.unit ?? '',
			statements: data.endurance?.statements ?? []
		}
	};
}

/** The figures the weekly chart can show, and the values the profile stores as
 *  a preference (#616) — one list so the two can't drift apart. `key` is the
 *  field on Week, `setting` the value the API expects. */
export const progressMetrics = [
	{
		key: 'avg_speed_kmh' as const,
		setting: 'speed',
		label: 'Tempo',
		color: 'var(--chart-speed)',
		format: (v: number) => `${v.toFixed(1)} km/h`
	},
	{
		key: 'distance_meters' as const,
		setting: 'distance',
		label: 'Kilometer',
		color: 'var(--chart-ctl)',
		format: (v: number) => `${Math.round(v / 1000)} km`
	},
	{
		key: 'moving_seconds' as const,
		setting: 'duration',
		label: 'Fahrzeit',
		color: 'var(--chart-power)',
		format: (v: number) => `${(v / 3600).toFixed(1)} h`
	},
	{
		key: 'elevation_gain_meters' as const,
		setting: 'elevation',
		label: 'Höhenmeter',
		color: 'var(--chart-elevation)',
		format: (v: number) => `${Math.round(v)} hm`
	}
];

export type ProgressMetricKey = (typeof progressMetrics)[number]['key'];

/** Maps a stored preference back to the weekly chart's field, so the chart
 *  opens on the figure the rider cares about. "load" has no weekly series, so
 *  it falls back to speed rather than showing nothing. */
export function metricKeyFor(setting: string | null): ProgressMetricKey {
	return progressMetrics.find((m) => m.setting === setting)?.key ?? 'avg_speed_kmh';
}
