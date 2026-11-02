// Syntax highlighting for files and Markdown code blocks: Shiki with its
// JavaScript regular expression engine, VS Code's Dark+ token colors, and the
// languages below. The page loads this module on first use.
import {createHighlighterCoreSync} from 'shiki/core';
import {createJavaScriptRegexEngine} from 'shiki/engine/javascript';
import darkPlus from 'shiki/themes/dark-plus.mjs';
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
  themes: [darkPlus],
  langs: [c, cpp, css, diff, dockerfile, go, html, ini, java, javascript, json, jsonc, jsonl, jsx,
    makefile, python, rust, scss, shellscript, sql, swift, toml, tsx, typescript, xml, yaml],
  engine: createJavaScriptRegexEngine(),
});

// Shiki names two of VS Code's languages differently.
const names = {javascriptreact: 'jsx', typescriptreact: 'tsx'};

const escape = text => text.replace(/[&<>"']/g, c => `&#${c.charCodeAt(0)};`);

// highlight returns the HTML of each line of text in a language, a VS Code
// language ID or a Markdown code block's language. Token text is escaped. An
// unknown language shows as plain text.
export function highlight(text, lang) {
  lang = names[lang] ?? lang;
  if (!highlighter.getLoadedLanguages().includes(lang)) lang = 'text';
  const {tokens} = highlighter.codeToTokens(text, {lang, theme: 'dark-plus'});
  return tokens.map(line => line.map(t => {
    const style = (t.color ? `color:${t.color};` : '') + (t.fontStyle & 1 ? 'font-style:italic;' : '') +
      (t.fontStyle & 2 ? 'font-weight:bold;' : '') + (t.fontStyle & 4 ? 'text-decoration:underline;' : '');
    return style ? `<span style="${style}">${escape(t.content)}</span>` : escape(t.content);
  }).join(''));
}
