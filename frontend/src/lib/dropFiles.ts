// Wails のファイルドロップ。ブラウザの File からはパスが得られないため、Go 側（internal/platform）が
// WindowFilesDropped を受けて `files-dropped`（ペイロード `{ files: string[] }`）を発行する。
// ドロップ先は `data-file-drop-target` 属性を持つ要素のみ（ComposePane のルート）。
export const FILES_DROPPED_EVENT = 'files-dropped'

/** イベントのペイロード（`{files}`、またはそれを 1 要素に包んだ配列）からパス一覧を取り出す。 */
export function droppedPaths(data: unknown): string[] {
  const payload = Array.isArray(data) && data.length === 1 && !isString(data[0]) ? data[0] : data
  const files = (payload as { files?: unknown } | null)?.files
  return Array.isArray(files) ? files.filter((f): f is string => isString(f) && f !== '') : []
}

function isString(v: unknown): v is string {
  return typeof v === 'string'
}
