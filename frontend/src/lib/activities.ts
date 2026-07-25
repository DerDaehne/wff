import { ApiError } from './api';

export interface UploadResult {
	activityId: number;
}

export async function uploadActivity(file: File): Promise<UploadResult> {
	const form = new FormData();
	form.append('file', file);
	const res = await fetch('/api/activities', { method: 'POST', body: form });
	if (!res.ok) {
		const text = await res.text().catch(() => '');
		throw new ApiError(res.status, text || res.statusText);
	}
	const json = await res.json();
	return { activityId: json.activity_id };
}

export function friendlyUploadError(err: unknown): string {
	if (err instanceof ApiError) {
		if (err.status === 409) return 'Diese Aktivität wurde bereits hochgeladen.';
		if (err.status === 413) return 'Datei ist zu groß.';
		if (err.status === 400) return 'Datei ist ungültig oder beschädigt (kein lesbares .fit-Format).';
		return err.message || 'Unbekannter Fehler beim Upload.';
	}
	if (err instanceof Error) return err.message;
	return 'Unbekannter Fehler beim Upload.';
}
