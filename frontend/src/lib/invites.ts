import { apiFetch } from './api';

/** A single-use, 72h registration link (#702) — any signed-in rider can mint
 *  one, there is no admin role. `token` is only ever returned here, once. */
export interface CreatedInvite {
	token: string;
	expires_at: string;
}

export async function createInvite(username: string, displayName: string): Promise<CreatedInvite> {
	const res = await apiFetch('/api/invites', {
		method: 'POST',
		headers: { 'content-type': 'application/json' },
		body: JSON.stringify({ username, display_name: displayName })
	});
	return res.json();
}
