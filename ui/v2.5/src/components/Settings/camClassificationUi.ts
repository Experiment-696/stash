export interface IClassificationCounts {
  matched: number;
  applied: number;
  skipped: number;
  conflicted: number;
}
export interface IClassificationDraft {
  name: string;
  pattern: string;
  category: string;
}
export function classificationDraftError(
  draft: IClassificationDraft
): string | undefined {
  if (!draft.name.trim()) return "Rule name is required.";
  if (!draft.pattern.trim()) return "Regular expression is required.";
  if (!draft.category.trim()) return "Cam Show category is required.";
  try {
    new RegExp(draft.pattern);
  } catch (error) {
    return error instanceof Error
      ? "Invalid regular expression: " + error.message
      : "Invalid regular expression.";
  }
  return undefined;
}
export function classificationCountsLabel(
  counts: IClassificationCounts
): string {
  return (
    counts.matched +
    " matched | " +
    counts.applied +
    " applied | " +
    counts.skipped +
    " skipped | " +
    counts.conflicted +
    " conflicts"
  );
}
export const classificationExamples = {
  basename: String.raw`^\d{4}-\d{2}-\d{2} \d{2}-\d{2}-\d{2}\.mp4$`,
  relativePath: String.raw`^captures/2026/.*\.mp4$`,
};

export const classificationApplyConfirmation =
  "Apply the current enabled rules? This changes Cam Show and scene-tag metadata only. Media files are never renamed, rewritten, or deleted. Reapplying is idempotent.";
