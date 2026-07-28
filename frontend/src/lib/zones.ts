import type { RideStatement } from './rides';

/** One heart-rate band with the time spent in it (#621). `key` doubles as the
 *  colour token name (`--zone-<key>`), so a band looks the same wherever it
 *  appears. */
export interface Zone {
	key: string;
	name: string;
	meaning: string;
	seconds: number;
	share: number;
}

export interface ZoneDistribution {
	zones: Zone[];
	total_seconds: number;
	statements: RideStatement[];
}

export const emptyZones: ZoneDistribution = { zones: [], total_seconds: 0, statements: [] };
