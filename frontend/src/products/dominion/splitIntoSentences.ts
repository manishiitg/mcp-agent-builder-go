// The workflow's rationale text is algorithmically generated: dense,
// semicolon/parenthesis-heavy, and rendered as one unbroken paragraph reads
// as an illegible wall of text. Splitting on sentence-ending punctuation
// followed by whitespace turns it into one line per clause, which is a
// legibility win even though it's a plain heuristic, not real parsing --
// there is no ". " (dot immediately followed by a space) anywhere in this
// dataset's decimal numbers (e.g. "0.27", "49.3"), so it doesn't
// mis-split those.
export function splitIntoSentences(text: string): string[] {
  return text.split(/(?<=[.!?])\s+/).map((s) => s.trim()).filter(Boolean)
}
