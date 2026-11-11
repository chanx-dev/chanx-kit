// A sentence ends at . ! ? or an ellipsis followed by space, or at a line break.
const BOUNDARY = /(?<=[.!?…])["')\]]*\s+|\n+/g;

/** Cuts streamed text into sentences; tiny ones wait for the next. */
export class SentenceBuffer {
  private text = '';

  constructor(private readonly minChars = 12) {}

  feed(delta: string): string[] {
    this.text += delta;
    const sentences: string[] = [];
    let start = 0;
    for (const match of this.text.matchAll(BOUNDARY)) {
      const end = match.index + match[0].length;
      const candidate = this.text.slice(start, end).trim();
      if ([...candidate].length >= this.minChars) {
        sentences.push(candidate);
        start = end;
      }
    }
    this.text = this.text.slice(start);
    return sentences;
  }

  /** Whatever is left, at the end of the reply. */
  flush(): string | undefined {
    const rest = this.text.trim();
    this.text = '';
    return rest || undefined;
  }
}
