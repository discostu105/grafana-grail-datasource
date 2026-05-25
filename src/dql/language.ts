// Monaco language registration for DQL — Dynatrace Query Language.
//
// Registers once globally via registerDqlLanguage(). Safe to call multiple
// times; subsequent calls are no-ops.
//
// Includes:
//   - Monarch tokenizer (verbs, operators, strings, numbers, durations,
//     comments).
//   - Bracket pairs + auto-closing for (, [, {, ", '.
//   - Completion provider stub — the real suggestions come from a backend
//     resource handler that proxies Grail's /query:autocomplete endpoint.

import type { languages, IPosition, editor } from 'monaco-editor';
import type { Monaco } from '@grafana/ui';

const LANGUAGE_ID = 'dql';
let registered = false;

// Verbs / commands — kept in sync with the dt-dql-essentials skill's
// "Commands" list.
const KEYWORDS = [
  'append', 'data', 'dedup', 'describe', 'expand', 'fetch', 'fields',
  'fieldsAdd', 'fieldsFlatten', 'fieldsKeep', 'fieldsRemove', 'fieldsRename',
  'fieldsSnapshot', 'fieldsSummary', 'filter', 'filterOut', 'join',
  'joinNested', 'limit', 'load', 'lookup', 'makeTimeseries', 'metrics',
  'parse', 'search', 'smartscapeEdges', 'smartscapeNodes', 'sort',
  'summarize', 'timeseries', 'traverse',
];

// Function names kept in a future-friendly export but not currently wired
// into the tokenizer (the monarch pattern recognises any `ident(` as
// type.identifier). Suggestion-list fallback uses the Grail autocomplete
// API instead.
const OPERATORS = [
  '==', '!=', '<=', '>=', '<', '>', '&&', '||', 'AND', 'OR', 'NOT', 'and', 'or', 'not',
  '+', '-', '*', '/', '=',
];

export const monarchLanguage: languages.IMonarchLanguage = {
  defaultToken: '',
  ignoreCase: false,
  keywords: KEYWORDS,
  operators: OPERATORS,
  symbols: /[=><!~?:&|+\-*/^%]+/,
  tokenizer: {
    root: [
      // Comments (DQL uses // line comments and /* block */)
      [/\/\/.*$/, 'comment'],
      [/\/\*/, { token: 'comment.quote', next: '@comment' }],
      // DQL macros (kept first so they highlight inside other contexts)
      [/\$__[A-Za-z_]+(\([^)]*\))?/, 'variable.predefined'],
      [/\$\{[A-Za-z_][^}]*\}/, 'variable'],
      [/\$[A-Za-z_][A-Za-z0-9_]*/, 'variable'],
      // Strings
      [/"([^"\\]|\\.)*$/, 'string.invalid'],
      [/"/, { token: 'string.quote', next: '@string_double' }],
      [/'([^'\\]|\\.)*$/, 'string.invalid'],
      [/'/, { token: 'string.quote', next: '@string_single' }],
      // Duration literals: 5m, 30s, 1h, 24h, 7d, 100ms
      [/\b\d+(?:\.\d+)?(?:ns|us|µs|ms|s|m|h|d|w|y)\b/, 'number.float'],
      // Numbers
      [/\b\d+\.\d+([eE][+-]?\d+)?\b/, 'number.float'],
      [/\b\d+([eE][+-]?\d+)?\b/, 'number'],
      // Identifiers — match function calls first so they highlight
      [/[a-zA-Z][\w.]*(?=\s*\()/, {
        cases: {
          '@keywords': 'keyword',
          '@default': 'type.identifier',
        },
      }],
      [/[a-zA-Z][\w.]*/, {
        cases: {
          '@keywords': 'keyword',
          '@default': 'identifier',
        },
      }],
      // Punctuation
      [/[{}()\[\]]/, '@brackets'],
      [/[;,.]/, 'delimiter'],
      [/@symbols/, {
        cases: {
          '@operators': 'operator',
          '@default': '',
        },
      }],
      [/\s+/, 'white'],
    ],
    comment: [
      [/[^/*]+/, 'comment'],
      [/\*\//, { token: 'comment.quote', next: '@pop' }],
      [/[/*]/, 'comment'],
    ],
    string_double: [
      [/[^\\"]+/, 'string'],
      [/\\./, 'string.escape'],
      [/"/, { token: 'string.quote', next: '@pop' }],
    ],
    string_single: [
      [/[^\\']+/, 'string'],
      [/\\./, 'string.escape'],
      [/'/, { token: 'string.quote', next: '@pop' }],
    ],
  },
};

export const languageConfig: languages.LanguageConfiguration = {
  comments: { lineComment: '//', blockComment: ['/*', '*/'] },
  brackets: [
    ['{', '}'],
    ['[', ']'],
    ['(', ')'],
  ],
  autoClosingPairs: [
    { open: '{', close: '}' },
    { open: '[', close: ']' },
    { open: '(', close: ')' },
    { open: '"', close: '"' },
    { open: "'", close: "'" },
  ],
  surroundingPairs: [
    { open: '{', close: '}' },
    { open: '[', close: ']' },
    { open: '(', close: ')' },
    { open: '"', close: '"' },
    { open: "'", close: "'" },
  ],
};

// Suggestion shape from Grail's /platform/storage/query/v1/query:autocomplete
// — we keep the keys we care about, ignore the rest.
type GrailSuggestion = {
  suggestion: string;
  alreadyTypedCharacters?: number;
  parts?: Array<{ type?: string; suggestion?: string; info?: string }>;
};
type GrailAutocompleteResponse = { suggestions: GrailSuggestion[] };

type AutocompleteFetcher = (dql: string, position: number) => Promise<GrailAutocompleteResponse>;

function suggestionKind(monaco: Monaco, parts: GrailSuggestion['parts']): languages.CompletionItemKind {
  const ck = monaco.languages.CompletionItemKind;
  const t = parts?.[0]?.type ?? '';
  switch (t) {
    case 'COMMAND':
    case 'KEYWORD':
      return ck.Keyword;
    case 'DATA_OBJECT':
      return ck.Class;
    case 'FIELD':
    case 'PARAMETER_KEY':
      return ck.Field;
    case 'FUNCTION':
      return ck.Function;
    case 'OPERATOR':
      return ck.Operator;
    case 'LITERAL':
    case 'STRING_LITERAL':
      return ck.Constant;
    default:
      return ck.Text;
  }
}

export function registerDqlLanguage(monaco: Monaco, fetcher: AutocompleteFetcher): void {
  if (registered) {
    return;
  }
  registered = true;

  monaco.languages.register({ id: LANGUAGE_ID });
  monaco.languages.setMonarchTokensProvider(LANGUAGE_ID, monarchLanguage);
  monaco.languages.setLanguageConfiguration(LANGUAGE_ID, languageConfig);

  monaco.languages.registerCompletionItemProvider(LANGUAGE_ID, {
    triggerCharacters: ['.', ':', ' ', ',', '{', '|', '$'],
    async provideCompletionItems(model: editor.ITextModel, position: IPosition) {
      const text = model.getValue();
      const offset = model.getOffsetAt(position);

      let response: GrailAutocompleteResponse;
      try {
        response = await fetcher(text, offset);
      } catch {
        // Fall back to a static keyword list if the proxy fails so the user
        // still gets *something*.
        return {
          suggestions: KEYWORDS.map((k) => ({
            label: k,
            kind: monaco.languages.CompletionItemKind.Keyword,
            insertText: k,
            range: model.getWordUntilPosition(position) && {
              startLineNumber: position.lineNumber,
              endLineNumber: position.lineNumber,
              startColumn: model.getWordUntilPosition(position).startColumn,
              endColumn: model.getWordUntilPosition(position).endColumn,
            },
          })) as languages.CompletionItem[],
        };
      }

      const word = model.getWordUntilPosition(position);
      const range = {
        startLineNumber: position.lineNumber,
        endLineNumber: position.lineNumber,
        startColumn: word.startColumn,
        endColumn: word.endColumn,
      };

      const items: languages.CompletionItem[] = [];
      for (const s of response.suggestions ?? []) {
        if (!s.suggestion) {
          continue;
        }
        items.push({
          label: s.suggestion,
          kind: suggestionKind(monaco, s.parts),
          insertText: s.suggestion,
          detail: s.parts?.[0]?.type ?? '',
          documentation: s.parts?.[0]?.info,
          range,
        });
      }
      return { suggestions: items };
    },
  });
}

export const DQL_LANGUAGE_ID = LANGUAGE_ID;
