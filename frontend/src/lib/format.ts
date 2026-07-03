/** Formats an ISO datetime string as a short date+time: "Jun 29, 14:32" */
export function fmtDateTime(iso: string): string {
	return new Date(iso).toLocaleString(undefined, {
		month: 'short',
		day: 'numeric',
		hour: '2-digit',
		minute: '2-digit'
	});
}

/** Formats a date-only string (YYYY-MM-DD) as a short date: "Jun 29" */
export function fmtDate(date: string): string {
	return new Date(date + 'T00:00:00').toLocaleDateString(undefined, {
		month: 'short',
		day: 'numeric'
	});
}

/** Formats an ISO datetime string as a date only: "Jun 29, 2025" */
export function fmtDateOnly(iso: string): string {
	return new Date(iso).toLocaleDateString(undefined, {
		year: 'numeric',
		month: 'short',
		day: 'numeric'
	});
}

/** Formats a duration in seconds as "42s" or "3m 7s". */
export function fmtDuration(seconds: number): string {
	if (seconds < 60) return `${Math.round(seconds)}s`;
	const m = Math.floor(seconds / 60);
	const s = Math.round(seconds % 60);
	return s > 0 ? `${m}m ${s}s` : `${m}m`;
}

/** Formats the duration between two ISO timestamps, or "-" if either is missing. */
export function fmtDurationBetween(start: string | null, end: string | null): string {
	if (!start || !end) return '-';
	const s = Math.round((new Date(end).getTime() - new Date(start).getTime()) / 1000);
	return fmtDuration(s);
}
