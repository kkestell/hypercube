// Syntax highlighting for files and Markdown code blocks: Shiki with its
// JavaScript regular expression engine, GitHub's default token colors, and the
// languages below. The page loads this module on first use.
import {createHighlighterCoreSync} from 'shiki/core';
import {createJavaScriptRegexEngine} from 'shiki/engine/javascript';
import githubDark from 'shiki/themes/github-dark-default.mjs';
import githubLight from 'shiki/themes/github-light-default.mjs';
import c from 'shiki/langs/c.mjs';
import cpp from 'shiki/langs/cpp.mjs';
import css from 'shiki/langs/css.mjs';
import diff from 'shiki/langs/diff.mjs';
import dockerfile from 'shiki/langs/dockerfile.mjs';
import go from 'shiki/langs/go.mjs';
import html from 'shiki/langs/html.mjs';
import ini from 'shiki/langs/ini.mjs';
import java from 'shiki/langs/java.mjs';
import javascript from 'shiki/langs/javascript.mjs';
import json from 'shiki/langs/json.mjs';
import jsonc from 'shiki/langs/jsonc.mjs';
import jsonl from 'shiki/langs/jsonl.mjs';
import jsx from 'shiki/langs/jsx.mjs';
import makefile from 'shiki/langs/makefile.mjs';
import markdown from 'shiki/langs/markdown.mjs';
import ocaml from 'shiki/langs/ocaml.mjs';
import python from 'shiki/langs/python.mjs';
import rust from 'shiki/langs/rust.mjs';
import scss from 'shiki/langs/scss.mjs';
import shellscript from 'shiki/langs/shellscript.mjs';
import sql from 'shiki/langs/sql.mjs';
import swift from 'shiki/langs/swift.mjs';
import toml from 'shiki/langs/toml.mjs';
import tsx from 'shiki/langs/tsx.mjs';
import typescript from 'shiki/langs/typescript.mjs';
import xml from 'shiki/langs/xml.mjs';
import yaml from 'shiki/langs/yaml.mjs';

const highlighter = createHighlighterCoreSync({
  themes: [githubDark, githubLight],
  langs: [c, cpp, css, diff, dockerfile, go, html, ini, java, javascript, json, jsonc, jsonl, jsx,
    makefile, markdown, ocaml, python, rust, scss, shellscript, sql, swift, toml, tsx, typescript, xml, yaml],
  engine: createJavaScriptRegexEngine(),
});

// Shiki names two of VS Code's languages differently.
const names: Record<string, string> = {javascriptreact: 'jsx', typescriptreact: 'tsx'};

const tokenClasses = [
  '[color:var(--shiki-light)] dark:[color:var(--shiki-dark)]',
  '[font-style:var(--shiki-light-font-style,normal)] dark:[font-style:var(--shiki-dark-font-style,normal)]',
  '[font-weight:var(--shiki-light-font-weight,normal)] dark:[font-weight:var(--shiki-dark-font-weight,normal)]',
  '[text-decoration:var(--shiki-light-text-decoration,none)] dark:[text-decoration:var(--shiki-dark-text-decoration,none)]',
].join(' ');

const escape = (text: string) => text.replace(/[&<>"']/g, c => `&#${c.charCodeAt(0)};`);

// highlight returns the HTML of each line of text in a language, a VS Code
// language ID or a Markdown code block's language. Token text is escaped. An
// unknown language shows as plain text.
export function highlight(text: string, lang: string): string[] {
  const named = names[lang] ?? lang;
  const {tokens} = highlighter.codeToTokens(text, {
    lang: highlighter.getLoadedLanguages().includes(named) ? named : 'text', themes: {light: githubLight.name, dark: githubDark.name}, defaultColor: false,
  });
  return tokens.map(line => line.map(t => {
    const style = Object.entries(t.htmlStyle || {}).map(([key, value]) => `${key}:${value}`).join(';');
    return style ? `<span class="${tokenClasses}" style="${style}">${escape(t.content)}</span>` : escape(t.content);
  }).join(''));
}
