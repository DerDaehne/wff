export class ApiError extends Error {
	status: number;
	constructor(status: number, message: string) {
		super(message);
		this.status = status;
	}
}

export async function apiFetch(input: string, init?: RequestInit): Promise<Response> {
	const res = await fetch(input, init);
	if (!res.ok) {
		const text = await res.text().catch(() => '');
		throw new ApiError(res.status, text || res.statusText);
	}
	return res;
}
