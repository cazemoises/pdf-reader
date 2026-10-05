import { test, after } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync, writeFileSync, mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { pathToFileURL } from 'node:url';
import ts from 'typescript';

const directory = mkdtempSync(join(tmpdir(), 'pdf-reader-tests-'));
after(() => rmSync(directory, { recursive: true, force: true }));
async function load(name) {
  const input = readFileSync(new URL(`../src/lib/${name}.ts`, import.meta.url), 'utf8');
  const output = ts.transpileModule(input, { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ESNext } }).outputText;
  const file = join(directory, `${name}.mjs`);
  writeFileSync(file, output);
  return import(pathToFileURL(file));
}
const { reflowPageText, paragraphOffsets } = await load('pageText');
const { segmentParagraph } = await load('highlightLayout');

test('legacy text keeps paragraph boundaries and existing highlight offsets', () => {
  const paragraphs = reflowPageText('Primeiro parágrafo.\n\nSegundo parágrafo.');
  assert.deepEqual(paragraphs, ['Primeiro parágrafo.', 'Segundo parágrafo.']);
  assert.deepEqual(paragraphOffsets(paragraphs), [0, 21]);
  const highlight = { id: 'old', pageNumber: 1, range: { start: 0, end: 8 }, color: 'yellow' };
  const segments = segmentParagraph(paragraphs[0], 0, [highlight]);
  assert.equal(segments[0].text, 'Primeiro');
  assert.equal(segments[0].highlight.id, 'old');
  assert.equal(segments.map(s => s.text).join(''), paragraphs[0]);
});
test('legacy hyphen and UTF-16 offset behavior remains unchanged', () => {
  assert.deepEqual(reflowPageText('palavra-\nquebrada'), ['palavraquebrada']);
  const paragraphs = reflowPageText('a\u0301\nlinha\n\n😀 próximo');
  assert.deepEqual(paragraphs, ['a\u0301 linha', '😀 próximo']);
  assert.equal(paragraphOffsets(paragraphs)[1], paragraphs[0].length + 2);
});
test('new compatible table text keeps rows separate in the existing reader', () => {
  assert.deepEqual(reflowPageText('Name | Value\n\nAlpha Beta | 42'), ['Name | Value', 'Alpha Beta | 42']);
});
