/*
Copyright (C) 2023-2026 QuantumNous

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
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import {
  classifyPreConsumeDraft,
  formatPreConsumeValue,
  MAX_TASK_PRE_CONSUME_TOKENS,
} from './async-pre-consume-token-field-helpers'

// 保护 P1 不变量：异步预扣上界字段只接受「空（清空）」或「正整数」。
// 0 / 负数 / 小数 / 非数字必须判为 invalid——非法输入绝不回写受控值，避免静默
// 删除既有 preconsume_tokens[model] 配置（见验收要求 9.2.6）。
describe('async pre-consume draft classification', () => {
  const cases: Array<{
    input: string
    expected: 'empty' | 'valid' | 'invalid'
  }> = [
    // 空串 = 清空（合法；保存时从映射删除该 model）
    { input: '', expected: 'empty' },
    { input: '   ', expected: 'empty' },
    // 正整数 = 合法
    { input: '1', expected: 'valid' },
    { input: '250000', expected: 'valid' },
    { input: '  250000  ', expected: 'valid' },
    { input: String(MAX_TASK_PRE_CONSUME_TOKENS), expected: 'valid' },
    // 0 = 非法（与「空」严格区分，不能提交）
    { input: '0', expected: 'invalid' },
    { input: '00', expected: 'invalid' },
    // 负数 = 非法
    { input: '-1', expected: 'invalid' },
    { input: '-250000', expected: 'invalid' },
    // 小数 = 非法（原先会被 Math.floor 静默截断）
    { input: '1.9', expected: 'invalid' },
    { input: '250000.5', expected: 'invalid' },
    // 超过后端统一上界或 JS 安全整数 = 非法
    { input: String(MAX_TASK_PRE_CONSUME_TOKENS + 1), expected: 'invalid' },
    { input: String(Number.MAX_SAFE_INTEGER + 1), expected: 'invalid' },
    // 非数字 = 非法
    { input: 'abc', expected: 'invalid' },
    { input: '250k', expected: 'invalid' },
  ]

  for (const { input, expected } of cases) {
    test(`classifyPreConsumeDraft(${JSON.stringify(input)}) === ${expected}`, () => {
      assert.equal(classifyPreConsumeDraft(input), expected)
    })
  }

  test('formatPreConsumeValue renders empty for non-positive / undefined', () => {
    assert.equal(formatPreConsumeValue(undefined), '')
    assert.equal(formatPreConsumeValue(0), '')
    assert.equal(formatPreConsumeValue(-5), '')
    assert.equal(formatPreConsumeValue(250000), '250000')
  })
})
