import type { RideStatement } from './rides';

/** One heart-rate band with the time spent in it (#621). */
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
	/** True when these zones come from an observed maximum heart rate instead
	 *  of a real threshold test (#624) — a looser stand-in the UI must say so
	 *  about. */
	assumed?: boolean;
}

export const emptyZones: ZoneDistribution = { zones: [], total_seconds: 0, statements: [] };

/** One band's colour key and share — the trimmed view a list row needs
 *  (#633), without the names/meanings a detail card carries. */
export interface ZoneShare {
	key: string;
	share: number;
}

export interface ZoneShares {
	zones: ZoneShare[];
	/** Same meaning as ZoneDistribution.assumed (#624). */
	assumed?: boolean;
}

/** The bands' names by key, as analyze/zones.go names them — a list row only
 *  gets keys and shares (#633). */
const zoneNames: Record<string, string> = {
	recovery: 'Ganz locker',
	endurance: 'Grundlage',
	tempo: 'Zügig',
	threshold: 'Schwelle',
	vo2: 'Ganz hart'
};

/** A ride's character in words, for the ride list: the band it spent most
 *  time in, plus how much was at threshold or harder ("meist Grundlage ·
 *  12 % hart"). Words instead of a coloured bar, since WFF has no colour code
 *  for zones while Rekta has none (ADR adr-003-wff-datenfarben-rekta). */
export function zoneCharacter(z: ZoneShares): string {
	const main = z.zones.reduce((a, b) => (b.share > a.share ? b : a));
	const hard = z.zones
		.filter((x) => x.key === 'threshold' || x.key === 'vo2')
		.reduce((sum, x) => sum + x.share, 0);
	const name = zoneNames[main.key] ?? main.key;
	return hard >= 0.05 ? `meist ${name} · ${Math.round(hard * 100)} % hart` : `meist ${name}`;
}
