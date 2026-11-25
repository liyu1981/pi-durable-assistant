import type { ReactNode } from "react";
import { makeKeyFactory } from "../lib/format";

const INLINE = /(`[^`\n]+`)|(\*\*[^*]+\*\*)|(\[[^\]\n]+\]\(https?:\/\/[^\s)]+\))/g;
const URL = /(https?:\/\/[^\s<)]+)/g;

function inline(text: string): ReactNode[] {
  const nodes: ReactNode[] = [];
  let last = 0;
  for (const match of text.matchAll(INLINE)) {
    const at = match.index ?? 0;
    if (at > last) nodes.push(...plain(text.slice(last, at)));
    const token = match[0];
    if (token.startsWith("`")) {
      nodes.push(<code key={at}>{token.slice(1, -1)}</code>);
    } else if (token.startsWith("**")) {
      nodes.push(<strong key={at}>{token.slice(2, -2)}</strong>);
    } else {
      const link = /^\[([^\]]+)\]\((https?:\/\/\S+)\)$/.exec(token);
      if (link) {
        nodes.push(
          <a key={at} href={link[2]} target="_blank" rel="noopener noreferrer">
            {link[1]}
          </a>,
        );
      }
    }
    last = at + token.length;
  }
  if (last < text.length) nodes.push(...plain(text.slice(last)));
  return nodes;
}

function plain(text: string): ReactNode[] {
  const nodes: ReactNode[] = [];
  let last = 0;
  for (const match of text.matchAll(URL)) {
    const at = match.index ?? 0;
    if (at > last) nodes.push(text.slice(last, at));
    nodes.push(
      <a key={`url-${at}`} href={match[0]} target="_blank" rel="noopener noreferrer">
        {match[0]}
      </a>,
    );
    last = at + match[0].length;
  }
  if (last < text.length) nodes.push(text.slice(last));
  return nodes;
}

/** Deliberately small, HTML-free Markdown renderer for model output. */
export function Markdown({ text }: { text: string }) {
  const lines = text.replace(/\r\n?/g, "\n").split("\n");
  const blocks: ReactNode[] = [];
  const keyFor = makeKeyFactory();
  let paragraph: string[] = [];
  let index = 0;

  const flush = () => {
    if (paragraph.length === 0) return;
    const content = paragraph;
    paragraph = [];
    blocks.push(
      <p key={`p-${index}`}>
        {content.map((line, offset) => (
          <span key={keyFor(`line-${line}`)}>
            {offset > 0 ? <br /> : null}
            {inline(line)}
          </span>
        ))}
      </p>,
    );
  };

  while (index < lines.length) {
    const line = lines[index];
    const fence = /^\s*```/.exec(line);
    if (fence) {
      flush();
      const code: string[] = [];
      index += 1;
      while (index < lines.length && !/^\s*```\s*$/.test(lines[index])) {
        code.push(lines[index]);
        index += 1;
      }
      blocks.push(
        <pre key={`code-${index}`}>
          <code>{code.join("\n")}</code>
        </pre>,
      );
    } else if (!line.trim()) {
      flush();
    } else {
      const heading = /^(#{1,6})\s+(.*)$/.exec(line);
      if (heading) {
        flush();
        const level = Math.min(6, heading[1].length + 2);
        const Heading = `h${level}` as "h3" | "h4" | "h5" | "h6";
        blocks.push(<Heading key={`h-${index}`}>{inline(heading[2])}</Heading>);
      } else if (/^\s*([-*+]|\d+[.)])\s+/.test(line)) {
        flush();
        const ordered = /^\s*\d/.test(line);
        const items: string[] = [];
        while (index < lines.length && /^\s*([-*+]|\d+[.)])\s+/.test(lines[index])) {
          items.push(lines[index].replace(/^\s*([-*+]|\d+[.)])\s+/, ""));
          index += 1;
        }
        blocks.push(
          ordered ? (
            <ol key={`list-${index}`}>
              {items.map((item) => (
                <li key={keyFor(`item-${item}`)}>{inline(item)}</li>
              ))}
            </ol>
          ) : (
            <ul key={`list-${index}`}>
              {items.map((item) => (
                <li key={keyFor(`item-${item}`)}>{inline(item)}</li>
              ))}
            </ul>
          ),
        );
        continue;
      } else {
        paragraph.push(line);
      }
    }
    index += 1;
  }
  flush();
  return <>{blocks}</>;
}
