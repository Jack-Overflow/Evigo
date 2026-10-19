import js from '@eslint/js';
import stylistic from '@stylistic/eslint-plugin';
import { defineConfig, globalIgnores } from 'eslint/config';
import globals from 'globals';
import tseslint from 'typescript-eslint';
import reactHooks from 'eslint-plugin-react-hooks';
import reactRefresh from 'eslint-plugin-react-refresh';

export default defineConfig([
  globalIgnores(['node_modules', '*.tsbuildinfo']),
  {
    files: ['**/*.{js,ts,tsx}'],
    plugins: { '@stylistic': stylistic },
    rules: {
      'no-nested-ternary': 'error',
      curly: ['error', 'all'],
      '@stylistic/padding-line-between-statements': [
        'error',
        { blankLine: 'always', prev: 'import', next: '*' },
        { blankLine: 'any', prev: 'import', next: 'import' },
        {
          blankLine: 'always',
          prev: '*',
          next: ['function', 'class', 'export'],
        },
        {
          blankLine: 'always',
          prev: ['function', 'class', 'export'],
          next: '*',
        },
        {
          blankLine: 'always',
          prev: '*',
          next: ['if', 'switch', 'try', 'return'],
        },
        { blankLine: 'always', prev: 'block-like', next: '*' },
      ],
    },
  },
  {
    files: ['**/*.{ts,tsx}'],
    extends: [js.configs.recommended, tseslint.configs.recommended],
    languageOptions: { globals: globals.browser },
  },
  {
    files: ['src/**/*.{ts,tsx}'],
    extends: [reactHooks.configs.flat.recommended],
  },
  {
    files: ['src/**/*.tsx'],
    ignores: ['src/**/*.test.tsx', 'src/components/ui/**'],
    extends: [reactRefresh.configs.vite],
  },
  {
    files: ['*.js', 'vite.config.ts'],
    extends: [js.configs.recommended],
    languageOptions: { globals: globals.node },
  },
]);
