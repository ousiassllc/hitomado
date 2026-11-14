/**
 * アカウントの絞り込みの次の状態。通常クリック=単一選択（選択済みの単一を押すと「全アカウント」へ）、
 * Ctrl/Cmd+クリック=複数選択に加える／外す。サイドバーとスマホのチップで同じ。
 */
export function nextAccountIds(current: readonly string[], id: string, multi: boolean): string[] {
  if (multi) return current.includes(id) ? current.filter((x) => x !== id) : [...current, id]
  if (current.length === 1 && current[0] === id) return []
  return [id]
}
