import { startAuthentication, startRegistration, WebAuthnError } from '@simplewebauthn/browser';
import { apiFetch, ApiError } from './api';

// go-webauthn wraps the spec options in {"publicKey": ...} — same top-level
// shape `navigator.credentials.create()/.get()` itself expects — but
// @simplewebauthn/browser wants just the inner object.
interface PublicKeyWrapper<T> {
	publicKey: T;
}

export async function registerPasskey(token: string): Promise<void> {
	const creation = await apiFetch(`/auth/invite/${encodeURIComponent(token)}`).then(
		(r) =>
			r.json() as Promise<PublicKeyWrapper<Parameters<typeof startRegistration>[0]['optionsJSON']>>
	);
	const attestation = await startRegistration({ optionsJSON: creation.publicKey });
	await apiFetch(`/auth/invite/${encodeURIComponent(token)}`, {
		method: 'POST',
		headers: { 'content-type': 'application/json' },
		body: JSON.stringify(attestation)
	});
}

export async function loginWithPasskey(username: string): Promise<void> {
	const assertion = await apiFetch('/auth/login/begin', {
		method: 'POST',
		headers: { 'content-type': 'application/json' },
		body: JSON.stringify({ username })
	}).then(
		(r) =>
			r.json() as Promise<
				PublicKeyWrapper<Parameters<typeof startAuthentication>[0]['optionsJSON']>
			>
	);
	const auth = await startAuthentication({ optionsJSON: assertion.publicKey });
	await apiFetch('/auth/login/finish', {
		method: 'POST',
		headers: { 'content-type': 'application/json' },
		body: JSON.stringify(auth)
	});
}

export async function logout(): Promise<void> {
	await apiFetch('/auth/logout', { method: 'POST' });
}

export async function whoAmI(): Promise<{ user_id: number } | null> {
	// A network failure (e.g. offline, app shell served from the SW cache)
	// is treated the same as "not logged in" here — the alternative would
	// be a third UI state just for "can't tell," which isn't worth it for
	// what's meant to be a basic offline shell, not full offline sessions.
	try {
		const res = await fetch('/api/me');
		if (!res.ok) return null;
		return await res.json();
	} catch {
		return null;
	}
}

/** Human-readable message for the invite/login form's error state. */
export function friendlyAuthError(err: unknown): string {
	if (err instanceof WebAuthnError) {
		if (err.code === 'ERROR_CEREMONY_ABORTED') {
			return 'Passkey-Vorgang abgebrochen.';
		}
		return err.message;
	}
	if (err instanceof ApiError) {
		if (err.status === 404) return 'Einladungslink ist ungültig oder abgelaufen.';
		if (err.status === 409) return 'Einladungslink wurde bereits verwendet.';
		if (err.status === 401) return 'Anmeldung fehlgeschlagen.';
		return err.message || 'Unbekannter Fehler.';
	}
	if (err instanceof Error) return err.message;
	return 'Unbekannter Fehler.';
}
