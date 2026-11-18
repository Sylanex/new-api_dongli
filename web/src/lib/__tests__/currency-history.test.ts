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
import i18next from 'i18next'
import { afterEach, beforeEach, expect, test } from 'vitest'

import {
  formatTopupCredit,
  formatTopupPayment,
} from '@/features/wallet/lib/billing'
import zh from '@/i18n/locales/zh.json'
import {
  formatCurrencyFromUSD,
  formatQuotaWithCurrency,
  formatCumulativeQuota,
  getCumulativeDisplayQuota,
  getChartQuota,
} from '@/lib/currency'
import { formatLogQuota } from '@/lib/format'
import { mapStatusDataToConfig } from '@/lib/status-query'
import { useSystemConfigStore } from '@/stores/system-config-store'

const cutoverTime = 1700000000

beforeEach(() => {
  useSystemConfigStore.setState(useSystemConfigStore.getInitialState(), true)
  useSystemConfigStore.getState().setConfig(
    mapStatusDataToConfig({
      quota_display_type: 'CUSTOM',
      custom_currency_symbol: '点',
      custom_currency_exchange_rate: 1,
      quota_per_unit: 1000000,
      points_migration_cutover_time: cutoverTime,
      points_migration_legacy_quota_per_unit: 500000,
    })
  )
})

afterEach(async () => {
  await i18next.changeLanguage('en')
  useSystemConfigStore.setState(useSystemConfigStore.getInitialState(), true)
  localStorage.clear()
})

test('old log costs retain their original quota unit after switching to points', () => {
  expect(formatLogQuota(500000, cutoverTime - 1)).toBe('Legacy credits 1')
  expect(formatLogQuota(500000, cutoverTime)).toBe('点 0.5')
  expect(formatLogQuota(500000)).toBe('点 0.5')
})

test('old top-up quantities show legacy credits without applying the new currency rate', () => {
  expect(formatCurrencyFromUSD(7.3, { recordedAt: cutoverTime - 1 })).toBe(
    'Legacy credits 7.3'
  )
  expect(formatQuotaWithCurrency(3650000, { recordedAt: cutoverTime })).toBe(
    '点 3.65'
  )
})

test('a status response without migration metadata clears a previously loaded cutover', () => {
  useSystemConfigStore.getState().setConfig(
    mapStatusDataToConfig({
      quota_display_type: 'USD',
      quota_per_unit: 500000,
    })
  )
  expect(formatLogQuota(500000, cutoverTime - 1)).toBe('$1')
})

test('an old order paid after the cutover shows its actual credited quota', () => {
  const record = {
    id: 1,
    user_id: 1,
    amount: 1,
    money: 7.33,
    trade_no: 'legacy-order',
    payment_method: 'alipay',
    create_time: cutoverTime - 1,
    complete_time: cutoverTime - 1,
    status: 'success' as const,
  }
  expect(formatTopupCredit(record)).toBe('Legacy credits 1')
  expect(
    formatTopupCredit({
      ...record,
      credited_quota: 7330000,
      complete_time: cutoverTime,
    })
  ).toBe('点 7.33')
})

test('Chinese history uses the published legacy-credit translation', async () => {
  i18next.addResourceBundle('zhCN', 'translation', zh.translation)
  await i18next.changeLanguage('zhCN')
  expect(formatLogQuota(500000, cutoverTime - 1)).toBe('旧额度 1')
})

test('payment history uses the actual gateway quote with two decimal places', () => {
  expect(formatTopupPayment({ money: 6.935, quoted_money: '6.93' })).toBe(
    '6.93'
  )
  expect(formatTopupPayment({ money: 7.33 })).toBe('7.33')
})

test('cumulative usage uses the recorded baseline rather than adding different units', () => {
  expect(formatCumulativeQuota(1500000, 500000)).toBe('点 8.3')
  expect(getCumulativeDisplayQuota(7000000, 5000000)).toBe(75000000)
  expect(formatCumulativeQuota(1000000, 0)).toBe('点 1')
})

test('missing cumulative baselines show raw quota instead of a misleading point total', () => {
  expect(getCumulativeDisplayQuota(1500000)).toBeUndefined()
  expect(formatCumulativeQuota(1500000)).toBe('1,500,000 Raw quota')
})

test('dated buckets add up to the same points as the wallet across the cutover', () => {
  expect(
    getChartQuota(5000000, cutoverTime - 1) +
      getChartQuota(2000000, cutoverTime)
  ).toBe(75000000)
  expect(formatCumulativeQuota(0, 500000)).toBe('点 6.8')
  useSystemConfigStore.getState().setConfig({
    pointsMigration: { cutoverTime: 0, legacyQuotaPerUnit: 500000 },
  })
  expect(getChartQuota(5000000, cutoverTime - 1)).toBe(5000000)
  expect(formatCumulativeQuota(7000000, 5000000)).toBe('点 7')
})
