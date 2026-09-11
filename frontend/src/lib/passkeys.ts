import { startRegistration } from '@simplewebauthn/browser';
import { apiFetch } from './api';

export interface Passkey {
	id: number;
	name: string;
	created_at: string;
	last_used_at: string | null;
}

export async function listPasskeys(): Promise<Passkey[]> {
	const res = await apiFetch('/api/passkeys');
	return res.json();
}

// go-webauthn wraps the spec options in {"publicKey": ...} — same shape
// $lib/webauthn.ts already unwraps for invite registration/login.
interface PublicKeyWrapper<T> {
	publicKey: T;
}

export async function addPasskey(name: string): Promise<Passkey> {
	const creation = await apiFetch('/api/passkeys/begin', {
		method: 'POST',
		headers: { 'content-type': 'application/json' },
		body: JSON.stringify({ name })
	}).then(
		(r) => r.json() as Promise<PublicKeyWrapper<Parameters<typeof startRegistration>[0]['optionsJSON']>>
	);
	const attestation = await startRegistration({ optionsJSON: creation.publicKey });
	const res = await apiFetch('/api/passkeys/finish', {
		method: 'POST',
		headers: { 'content-type': 'application/json' },
		body: JSON.stringify(attestation)
	});
	return res.json();
}

export async function deletePasskey(id: number): Promise<void> {
	await apiFetch(`/api/passkeys/${id}`, { method: 'DELETE' });
}
