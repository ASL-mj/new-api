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

// 阶梯计费表达式解析（纯函数，无 DOM / UI 依赖）。
// 表达式语法与语义见 pkg/billingexpr/expr.md。

import {
  BILLING_VAR_REGEX,
  BILLING_VAR_KEY_TO_FIELD,
} from '../constants/billing.constants';

export function stripExprVersion(exprStr) {
  if (!exprStr) return { version: 1, body: '' };
  const m = exprStr.match(/^v(\d+):([\s\S]*)$/);
  if (m) return { version: Number(m[1]), body: m[2] };
  return { version: 1, body: exprStr };
}

function parseTierBody(bodyStr) {
  const coeffs = {};
  const re = new RegExp(BILLING_VAR_REGEX.source, 'g');
  let m;
  while ((m = re.exec(bodyStr)) !== null) {
    if (!(m[1] in coeffs)) coeffs[m[1]] = Number(m[2]);
  }
  const tier = {};
  for (const [varName, field] of Object.entries(BILLING_VAR_KEY_TO_FIELD)) {
    tier[field] = coeffs[varName] || 0;
  }
  return tier;
}

export function parseTiersFromExpr(exprStr) {
  if (!exprStr) return [];
  try {
    const { body } = stripExprVersion(exprStr);
    const condGroup = `((?:(?:p|c|len)\\s*(?:<|<=|>|>=)\\s*[\\d.eE+]+)(?:\\s*&&\\s*(?:p|c|len)\\s*(?:<|<=|>|>=)\\s*[\\d.eE+]+)*)`;
    const tierRe = new RegExp(
      `(?:${condGroup}\\s*\\?\\s*)?tier\\("([^"]*)",\\s*([^)]+)\\)`,
      'g',
    );
    const tiers = [];
    let m;
    while ((m = tierRe.exec(body)) !== null) {
      const condStr = m[1] || '';
      const conditions = [];
      if (condStr) {
        for (const cp of condStr.split(/\s*&&\s*/)) {
          const cm = cp.trim().match(/^(p|c|len)\s*(<|<=|>|>=)\s*([\d.eE+]+)$/);
          if (cm)
            conditions.push({ var: cm[1], op: cm[2], value: Number(cm[3]) });
        }
      }
      const tier = parseTierBody(m[3]);
      tier.label = m[2];
      tier.conditions = conditions;
      tiers.push(tier);
    }
    return tiers;
  } catch {
    return [];
  }
}
