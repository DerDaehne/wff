import { apiFetch } from './api';
import type { RideStory } from './rides';

export interface DayLoad {
	date: string;
	tss: number;
	ctl: number;
	atl: number;
	tsb: number;
}

export interface Insight {
	message: string;
	severity: 'info' | 'success' | 'warning';
}

export interface TrainingLoad {
	series: DayLoad[];
	insights: Insight[];
	/** "How am I doing?" in plain language — same shape as a ride's story, so
	 *  the dashboard and the ride detail present it identically (#602). */
	status: RideStory;
}

export async function getTrainingLoad(): Promise<TrainingLoad> {
	const res = await apiFetch('/api/training-load');
	const data = await res.json();
	// Go's JSON encoding renders a nil slice as `null`, not `[]` — no
	// activities yet (or none with a computable TSS) hits this.
	return {
		series: data.series ?? [],
		insights: data.insights ?? [],
		status: {
			...data.status,
			stats: data.status?.stats ?? [],
			statements: data.status?.statements ?? []
		}
	};
}
