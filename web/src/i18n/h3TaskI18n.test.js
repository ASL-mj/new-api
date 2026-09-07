/*
Copyright (C) 2025 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

import { describe, expect, test } from 'bun:test';
import { readFileSync } from 'node:fs';
import ts from 'typescript';

const sourceFiles = [
  '../components/table/task-logs/H3GenerationModal.jsx',
  '../components/table/task-logs/TaskBillingDetails.jsx',
  '../components/table/task-logs/modals/TaskDetailModal.jsx',
  '../components/table/task-logs/TaskLogsActions.jsx',
  '../components/table/task-logs/TaskLogsColumnDefs.jsx',
  '../pages/Setting/Ratio/H3PricingSettings.jsx',
  '../helpers/h3Task.js',
];
const keys = new Set(['任务账务', 'MiniMax-H3 定价']);
for (const path of sourceFiles) {
  const source = ts.createSourceFile(
    path,
    readFileSync(new URL(path, import.meta.url), 'utf8'),
    ts.ScriptTarget.Latest,
    true,
    ts.ScriptKind.JSX,
  );
  const visit = (node) => {
    if (
      (ts.isCallExpression(node) && node.expression.getText(source) === 't') ||
      (ts.isNewExpression(node) && node.expression.getText(source) === 'Error')
    ) {
      const argument = node.arguments?.[0];
      if (argument && ts.isStringLiteral(argument)) keys.add(argument.text);
    }
    ts.forEachChild(node, visit);
  };
  visit(source);
}

describe('H3 task translations', () => {
  test.each(['en', 'fr', 'ja', 'ru', 'vi', 'zh-CN', 'zh-TW'])(
    '%s covers H3 labels and validation messages',
    (lang) => {
      const resource = JSON.parse(
        readFileSync(
          new URL('./locales/' + lang + '.json', import.meta.url),
          'utf8',
        ),
      ).translation;
      for (const key of keys) {
        expect(typeof resource[key], lang + ': ' + key).toBe('string');
        expect(resource[key].trim(), lang + ': ' + key).not.toBe('');
      }
    },
  );
});
