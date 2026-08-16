// Extracted from App.tsx so the file viewer can be shared between the main
// app layout and a product surface (e.g. Video Studio) without duplicating
// this lookup table in two places.
const CODE_EXTENSIONS: Record<string, string> = {
  'go': 'go',
  'py': 'python',
  'ts': 'typescript',
  'tsx': 'typescript',
  'js': 'javascript',
  'jsx': 'javascript',
  'java': 'java',
  'c': 'c',
  'cpp': 'cpp',
  'cc': 'cpp',
  'cxx': 'cpp',
  'cs': 'csharp',
  'php': 'php',
  'rb': 'ruby',
  'sql': 'sql',
  'html': 'html',
  'htm': 'html',
  'css': 'css',
  'scss': 'scss',
  'sass': 'sass',
  'sh': 'shell',
  'bash': 'shell',
  'zsh': 'shell',
  'yaml': 'yaml',
  'yml': 'yaml',
  'xml': 'xml',
  'vue': 'vue',
  'svelte': 'svelte',
  'rs': 'rust',
  'swift': 'swift',
  'kt': 'kotlin',
  'scala': 'scala',
  'r': 'r',
  'lua': 'lua',
  'pl': 'perl',
  'dart': 'dart',
  'ex': 'elixir',
  'exs': 'elixir',
  'clj': 'clojure',
  'hs': 'haskell',
  'ml': 'ocaml',
  'fs': 'fsharp',
  'vb': 'vbnet',
  'ps1': 'powershell',
  'dockerfile': 'dockerfile',
  'makefile': 'makefile',
  'mk': 'makefile',
}

export function getCodeFileLanguage(filepath: string): string | null {
  const ext = filepath.toLowerCase().split('.').pop() || ''
  return CODE_EXTENSIONS[ext] || null
}

export function isCodeFile(filepath: string): boolean {
  return getCodeFileLanguage(filepath) !== null
}
