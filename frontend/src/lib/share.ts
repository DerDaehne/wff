import { apiFetch } from './api';

/** Whether a ride currently has an active share link, for the owner's own
 *  "shared" badge and copy/revoke controls (#641). */
export interface ShareStatus {
	active: boolean;
	token?: string;
	created_at?: string;
}

export async function getShareStatus(activityId: number): Promise<ShareStatus> {
	const res = await apiFetch(`/api/activities/${activityId}/share`);
	return res.json();
}

export async function createShare(activityId: number): Promise<ShareStatus> {
	const res = await apiFetch(`/api/activities/${activityId}/share`, { method: 'POST' });
	return res.json();
}

export async function revokeShare(activityId: number): Promise<ShareStatus> {
	const res = await apiFetch(`/api/activities/${activityId}/share`, { method: 'DELETE' });
	return res.json();
}

/** The stats-only view a share link shows to someone without a login —
 *  deliberately not the ride's full story (no map, no calorie/profile text). */
export interface PublicRideSummary {
	started_at: string;
	sport: string;
	moving_seconds: number;
	distance_meters: number | null;
	elevation_gain_meters: number | null;
	training_stress_score: number | null;
}

export async function getPublicShare(token: string): Promise<PublicRideSummary> {
	const res = await apiFetch(`/api/share/${token}`);
	return res.json();
}
