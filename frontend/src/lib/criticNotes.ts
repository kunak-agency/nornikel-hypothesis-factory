// criticNotes приходит ОДНОЙ плоской строкой со встроенными маркерами судей:
// [технолог] … [экономист] … [рецензент новизны] …
// Любой судья может «промолчать», поэтому блоков может быть 0–3.
// Разбиваем защитно, не рассчитывая на гарантированные три.

export interface CriticNote {
  role: string | null
  text: string
}

const RE = /\[(технолог|экономист|рецензент новизны)\]/gi

export function parseCriticNotes(raw?: string | null): CriticNote[] {
  if (!raw || !raw.trim()) return []
  const parts = raw.split(RE)
  const notes: CriticNote[] = []

  // Ведущий текст до первого маркера (если есть).
  const lead = parts[0]?.trim()
  if (lead) notes.push({ role: null, text: lead })

  for (let i = 1; i < parts.length; i += 2) {
    const role = parts[i]
    const text = (parts[i + 1] ?? '').trim()
    if (text) notes.push({ role: role.toLowerCase(), text })
  }
  return notes
}
