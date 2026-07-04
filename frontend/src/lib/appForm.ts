/** Form-data parsers shared by the application create and edit actions. */
import type { SkipConditions } from '$lib/types';

/** Parses the trigger fields emitted by TriggerFields.svelte. */
export function parseTrigger(data: FormData): {
	TriggerType: 'push' | 'tag';
	TagPattern: string;
	TagPrefix: string;
} {
	const raw = ((data.get('trigger_type') as string) ?? 'push').trim();
	const triggerType: 'push' | 'tag' = raw === 'tag' ? 'tag' : 'push';
	const tagPattern = ((data.get('tag_pattern') as string) ?? '').trim();
	const tagPrefix = ((data.get('tag_prefix') as string) ?? '').trim();
	return {
		TriggerType: triggerType,
		// Clear the field of the other trigger type so switching types does
		// not leave stale config behind.
		TagPattern: triggerType === 'tag' ? tagPattern : '',
		TagPrefix: triggerType === 'push' ? tagPrefix : ''
	};
}

/** Parses the notification hidden fields emitted by NotificationFields.svelte. */
export function parseNotifications(data: FormData): Record<string, unknown> {
	const onFailureUrl = (data.get('on_failure_url') as string)?.trim() ?? '';
	const onApprovalUrl = (data.get('on_approval_url') as string)?.trim() ?? '';
	let headers: Record<string, string> = {};
	try {
		headers = JSON.parse((data.get('notification_headers') as string) ?? '{}');
	} catch {
		/* ignore parse errors, send empty */
	}
	return {
		...(onFailureUrl ? { on_failure_url: onFailureUrl } : {}),
		...(onApprovalUrl ? { on_approval_url: onApprovalUrl } : {}),
		...(Object.keys(headers).length ? { headers } : {})
	};
}

/** Parses the skip-condition hidden fields emitted by SkipConditionsFields.svelte. */
export function parseSkipConditions(data: FormData): SkipConditions {
	const skipConditions: SkipConditions = {};
	try {
		const commitPatterns: string[] = JSON.parse((data.get('skip_commit_patterns') as string) ?? '[]');
		const pathsIgnore: string[] = JSON.parse((data.get('skip_paths_ignore') as string) ?? '[]');
		const pathsInclude: string[] = JSON.parse((data.get('skip_paths_include') as string) ?? '[]');
		if (commitPatterns.length) skipConditions.commit_patterns = commitPatterns;
		if (pathsIgnore.length) skipConditions.paths_ignore = pathsIgnore;
		if (pathsInclude.length) skipConditions.paths_include = pathsInclude;
		if (data.get('skip_backfill') === 'true') skipConditions.skip_backfill = true;
	} catch {
		/* ignore parse errors, send empty */
	}
	return skipConditions;
}
