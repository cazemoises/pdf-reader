import type { CharRange, Highlight } from "../api/types";

export interface TextSegment {
  text: string;
  highlight: Highlight | null;
}

// Splits a single paragraph's text into segments so each one can be
// rendered as plain text or as a <mark> for a highlight, given absolute
// character offsets (paragraphStart is this paragraph's offset in the
// same canonical offset space as paragraphOffsets() in pageText.ts).
export function segmentParagraph(
  paragraphText: string,
  paragraphStart: number,
  highlights: Highlight[],
): TextSegment[] {
  const paragraphEnd = paragraphStart + paragraphText.length;
  const overlapping = highlights.filter(
    (h) => h.range.start < paragraphEnd && h.range.end > paragraphStart,
  );

  const boundaries = new Set<number>([0, paragraphText.length]);
  for (const h of overlapping) {
    boundaries.add(Math.max(0, h.range.start - paragraphStart));
    boundaries.add(Math.min(paragraphText.length, h.range.end - paragraphStart));
  }
  const points = Array.from(boundaries).sort((a, b) => a - b);

  const segments: TextSegment[] = [];
  for (let i = 0; i < points.length - 1; i++) {
    const from = points[i];
    const to = points[i + 1];
    if (from === to) {
      continue;
    }
    const absoluteFrom = paragraphStart + from;
    const absoluteTo = paragraphStart + to;
    const covering =
      overlapping.find((h) => h.range.start <= absoluteFrom && h.range.end >= absoluteTo) ?? null;
    segments.push({ text: paragraphText.slice(from, to), highlight: covering });
  }
  return segments;
}

// Converts the current window selection (assumed to be inside `container`,
// whose direct children are one element per paragraph, in the same order
// as the paragraphs array used to render them) into a CharRange in the
// same canonical offset space as paragraphOffsets(). Only handles
// selections that start/end inside a text node, which covers ordinary
// click-and-drag text selection in every real browser; selections whose
// boundary lands exactly on an element (rare, mostly synthetic) are not
// supported and return null.
export function selectionToRange(container: HTMLElement, selection: Selection): CharRange | null {
  if (selection.rangeCount === 0) {
    return null;
  }
  const domRange = selection.getRangeAt(0);
  if (domRange.collapsed) {
    return null;
  }

  const start = domPositionToOffset(container, domRange.startContainer, domRange.startOffset);
  const end = domPositionToOffset(container, domRange.endContainer, domRange.endOffset);
  if (start === null || end === null || end <= start) {
    return null;
  }
  return { start, end };
}

function domPositionToOffset(container: HTMLElement, targetNode: Node, targetOffset: number): number | null {
  let offset = 0;

  const paragraphs = Array.from(container.children);
  for (let index = 0; index < paragraphs.length; index++) {
    const paragraph = paragraphs[index];
    if (index > 0) {
      offset += 2; // the "\n\n" paragraph separator, not present as DOM text
    }

    if (paragraph.contains(targetNode)) {
      const walker = document.createTreeWalker(paragraph, NodeFilter.SHOW_TEXT);
      let node = walker.nextNode();
      while (node) {
        if (node === targetNode) {
          return offset + targetOffset;
        }
        offset += node.textContent?.length ?? 0;
        node = walker.nextNode();
      }
      return offset;
    }

    offset += paragraph.textContent?.length ?? 0;
  }

  return null;
}

// HIGHLIGHT_COLORS in ReaderPage.tsx are solid hex swatches meant for the
// small color-picker dots; used directly as a <mark> background they'd be
// too saturated behind body text, so this converts to a translucent rgba
// for the actual highlight fill.
export function highlightBackground(hex: string): string {
  const r = parseInt(hex.slice(1, 3), 16);
  const g = parseInt(hex.slice(3, 5), 16);
  const b = parseInt(hex.slice(5, 7), 16);
  return `rgba(${r}, ${g}, ${b}, 0.35)`;
}
