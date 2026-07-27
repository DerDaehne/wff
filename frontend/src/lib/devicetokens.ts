import { apiFetch } from './api';

/** A token that lets one device upload rides without a browser session (#617).
 *  `token` is set exactly once, in the answer to create() — afterwards only the
 *  hash exists on the server and it can never be shown again. */
export interface DeviceToken {
	id: number;
	name: string;
	created_at: string;
	last_used_at: string | null;
	token?: string;
}

export async function listDeviceTokens(): Promise<DeviceToken[]> {
	const res = await apiFetch('/api/device-tokens');
	return res.json();
}

export async function createDeviceToken(name: string): Promise<DeviceToken> {
	const res = await apiFetch('/api/device-tokens', {
		method: 'POST',
		headers: { 'content-type': 'application/json' },
		body: JSON.stringify({ name })
	});
	return res.json();
}

export async function revokeDeviceToken(id: number): Promise<void> {
	await apiFetch(`/api/device-tokens/${id}`, { method: 'DELETE' });
}
