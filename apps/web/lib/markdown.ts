/**
 * Tiny no-dep Markdown renderer. The changelog file is structurally simple:
 * `##` headings followed by paragraphs. We do not want to pull a whole MDX
 * toolchain for two pages. When the scope grows we can swap to unified/remark.
 */
export type MdBlock =
  | { kind: "h2"; text: string }
  | { kind: "p"; text: string };

export function parseSimpleMarkdown(src: string): MdBlock[] {
  const blocks: MdBlock[] = [];
  const lines = src.split("\n");
  let buffer: string[] = [];
  const flush = () => {
    if (buffer.length) {
      const text = buffer.join("\n").trim();
      if (text) blocks.push({ kind: "p", text });
      buffer = [];
    }
  };
  for (const line of lines) {
    if (line.startsWith("## ")) {
      flush();
      blocks.push({ kind: "h2", text: line.slice(3).trim() });
    } else if (line.trim() === "") {
      flush();
    } else {
      buffer.push(line);
    }
  }
  flush();
  return blocks;
}
