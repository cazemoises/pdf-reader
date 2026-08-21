// Extractor blocks are frequently one PDF line each (see
// backend/internal/adapters/httpextractor: blocks are joined with "\n"),
// so a page's raw text is full of the source PDF's hard line breaks -
// including ones that split a single word across two lines with a
// trailing hyphen. reflowPageText joins any line ending in "-" with the
// next line when that next line starts with a lowercase letter (the
// common case for a word broken mid-syllable) and otherwise treats a line
// break as a mid-paragraph wrap, collapsing it into a single space so the
// browser reflows the text instead of keeping the PDF's fixed line
// geometry. A blank line in the source is kept as a paragraph break. This
// is a simple heuristic, not a dictionary-backed one: a legitimate
// end-of-line hyphen (e.g. a compound word) gets merged too, which is an
// accepted trade-off for a first version.
export function reflowPageText(raw: string): string[] {
  const rawLines = raw.split("\n");

  const mergedLines: string[] = [];
  for (const line of rawLines) {
    const previous = mergedLines[mergedLines.length - 1];
    if (previous !== undefined && previous.endsWith("-") && /^[a-z]/.test(line)) {
      mergedLines[mergedLines.length - 1] = previous.slice(0, -1) + line;
    } else {
      mergedLines.push(line);
    }
  }

  const paragraphs: string[] = [];
  let current: string[] = [];
  for (const line of mergedLines) {
    if (line.trim() === "") {
      if (current.length > 0) {
        paragraphs.push(current.join(" ").trim());
        current = [];
      }
      continue;
    }
    current.push(line.trim());
  }
  if (current.length > 0) {
    paragraphs.push(current.join(" ").trim());
  }

  return paragraphs.length > 0 ? paragraphs : [""];
}

// The canonical offset space highlights are anchored to: paragraphs
// joined with a 2-character separator ("\n\n"), even though that
// separator is never rendered as literal text - see highlightLayout.ts,
// which walks the DOM using this same +2-per-paragraph-boundary rule so
// offsets computed from a live selection match offsets computed here.
export function paragraphOffsets(paragraphs: string[]): number[] {
  const offsets: number[] = [];
  let running = 0;
  for (const paragraph of paragraphs) {
    offsets.push(running);
    running += paragraph.length + 2;
  }
  return offsets;
}
