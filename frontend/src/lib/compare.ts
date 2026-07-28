import { apiFetch } from './api';

/** One opted-in rider's relative training-success change — never an
 *  absolute figure like kilometres (#642). null means not enough history
 *  yet, an honest absence rather than a zero pretending to be an answer. */
export interface CompareEntry {
	display_name: string;
	is_you: boolean;
	delta_ctl: number | null;
}

export interface CompareResponse {
	/** False exactly when the caller themselves hasn't opted in — seeing
	 *  requires being seen, so there is nothing to show otherwise. */
	opted_in: boolean;
	entries: CompareEntry[];
}

export async function getCompare(): Promise<CompareResponse> {
	const res = await apiFetch('/api/compare');
	return res.json();
}
