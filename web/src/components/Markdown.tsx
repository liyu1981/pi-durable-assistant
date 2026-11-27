import type { ReactNode } from "react";
import { parseArtifactRef } from "../lib/artifact";
import { ArtifactEmbed, ArtifactInlineImage, ArtifactPlainLink } from "./ArtifactViewer";

const INLINE = /(!?\[[^\]\n]*\]\([^\s)]+\))|(`[^`\n]+`)|(\*\*[^*]+\*\*)|(artifact:\/\/[^\s<)]+)/g;
const URL_PATTERN = /(https?:\/\/[^\s<)]+)/g;

function renderUrls(text: string, base: string): ReactNode[] {
  const nodes: ReactNode[] = [];
  let last = 0;
  for (const match of text.matchAll(URL_PATTERN)) {
    const at = match.index ?? 0;
    if (at > last) nodes.push(text.slice(last, at));
    nodes.push(
      <a
        key={`${base}-url-${at}`}
        href={match[0]}
        target="_blank"
        rel="noopener noreferrer"
        className="text-primary underline underline-offset-2"
      >
        {match[0]}
      </a>,
    );
    last = at + match[0].length;
  }
  if (last < text.length) nodes.push(text.slice(last));
  return nodes;
}

function renderInline(text: string, base: string): ReactNode[] {
  const nodes: ReactNode[] = [];
  let last = 0;
  for (const match of text.matchAll(INLINE)) {
    const at = match.index ?? 0;
    if (at > last) nodes.push(...renderUrls(text.slice(last, at), `${base}-plain-${at}`));
    const token = match[0];
    if (token.startsWith("`")) {
      nodes.push(
        <code key={`${base}-code-${at}`} className="rounded bg-muted px-1 py-0.5 font-mono text-sm">
          {token.slice(1, -1)}
        </code>,
      );
    } else if (token.startsWith("**")) {
      nodes.push(
        <strong key={`${base}-bold-${at}`} className="font-semibold text-foreground">
          {token.slice(2, -2)}
        </strong>,
      );
    } else if (token.startsWith("artifact://")) {
      const ref = parseArtifactRef(token);
      nodes.push(
        ref ? (
          <ArtifactPlainLink key={`${base}-artifact-${at}`} ref={ref}>
            {token}
          </ArtifactPlainLink>
        ) : (
          <span key={`${base}-artifact-${at}`}>{token}</span>
        ),
      );
    } else {
      const image = /^!\[([^\]]*)\]\(([^\s)]+)\)$/.exec(token);
      if (image) {
        const ref = parseArtifactRef(image[2]);
        nodes.push(
          ref ? (
            <ArtifactInlineImage key={`${base}-image-${at}`} ref={ref} alt={image[1]} />
          ) : (
            <img
              key={`${base}-image-${at}`}
              src={image[2]}
              alt={image[1]}
              className="max-h-80 max-w-full rounded-xl"
            />
          ),
        );
      } else {
        const link = /^\[([^\]]+)\]\(([^\s)]+)\)$/.exec(token);
        if (link) {
          const ref = parseArtifactRef(link[2]);
          if (ref) {
            nodes.push(<ArtifactEmbed key={`${base}-artifact-${at}`} ref={ref} />);
          } else {
            nodes.push(
              <a
                key={`${base}-link-${at}`}
                href={link[2]}
                target="_blank"
                rel="noopener noreferrer"
                className="text-primary underline underline-offset-2"
              >
                {link[1]}
              </a>,
            );
          }
        }
      }
    }
    last = at + token.length;
  }
  if (last < text.length) nodes.push(...renderUrls(text.slice(last), `${base}-tail`));
  return nodes;
}

function createKeyFactory(): (base: string) => string {
  const counts = new Map<string, number>();
  return (base: string) => {
    const count = (counts.get(base) ?? 0) + 1;
    counts.set(base, count);
    return count === 1 ? base : `${base}-${count}`;
  };
}

/** A deliberately small, HTML-free Markdown renderer for model output. */
export function Markdown({ text }: { text: string }) {
  const keyFor = createKeyFactory();
  const lines = text.replace(/\r\n?/g, "\n").split("\n");
  const blocks: ReactNode[] = [];
  const paragraph: string[] = [];
  let index = 0;

  const flush = () => {
    if (paragraph.length === 0) return;
    const content = [...paragraph];
    paragraph.length = 0;
    blocks.push(
      <p key={`p-${index}`} className="text-sm leading-6 text-foreground">
        {content.map((line, offset) => {
          const lineKey = keyFor("line");
          return (
            <span key={lineKey}>
              {offset > 0 ? <br /> : null}
              {renderInline(line, lineKey)}
            </span>
          );
        })}
      </p>,
    );
  };

  while (index < lines.length) {
    const line = lines[index];
    if (/^\s*```/.test(line)) {
      flush();
      const code: string[] = [];
      index += 1;
      while (index < lines.length && !/^\s*```\s*$/.test(lines[index])) {
        code.push(lines[index]);
        index += 1;
      }
      blocks.push(
        <pre
          key={`code-${index}`}
          className="overflow-x-auto rounded-xl border border-border bg-muted/50 p-3 text-sm"
        >
          <code className="font-mono">{code.join("\n")}</code>
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
        blocks.push(
          <Heading key={`h-${index}`} className="text-sm font-semibold text-foreground">
            {renderInline(heading[2], `h-${index}`)}
          </Heading>,
        );
      } else if (/^\s*([-*+]|\d+[.)])\s+/.test(line)) {
        flush();
        const ordered = /^\s*\d/.test(line);
        const items: string[] = [];
        while (index < lines.length && /^\s*([-*+]|\d+[.)])\s+/.test(lines[index])) {
          items.push(lines[index].replace(/^\s*([-*+]|\d+[.)])\s+/, ""));
          index += 1;
        }
        const list = items.map((item) => {
          const itemKey = keyFor("item");
          return <li key={itemKey}>{renderInline(item, itemKey)}</li>;
        });
        blocks.push(
          ordered ? (
            <ol key={`list-${index}`} className="ml-5 list-decimal space-y-1 text-sm">
              {list}
            </ol>
          ) : (
            <ul key={`list-${index}`} className="ml-5 list-disc space-y-1 text-sm">
              {list}
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
