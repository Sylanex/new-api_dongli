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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { render, screen } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'
import { useSystemConfigStore } from '@/stores/system-config-store'

import { CommonLogsStats } from '../common-logs-stats'
import { UsageLogsProvider } from '../usage-logs-provider'

afterEach(() => {
  useSystemConfigStore.setState(useSystemConfigStore.getInitialState(), true)
  localStorage.clear()
})

test('cross-cutover statistics display point usage and legacy usage separately', async () => {
  const state = useSystemConfigStore.getState()
  state.setConfig({
    currency: {
      ...state.config.currency,
      quotaDisplayType: 'CUSTOM',
      customCurrencySymbol: '点',
    },
    pointsMigration: { cutoverTime: 1700000000, legacyQuotaPerUnit: 500000 },
  })
  vi.spyOn(api, 'get').mockResolvedValue({
    data: {
      success: true,
      data: { quota: 1500000, legacy_quota: 500000, rpm: 2, tpm: 10 },
    },
  })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const root = createRootRoute()
  const auth = createRoute({ getParentRoute: () => root, id: '_authenticated' })
  const logs = createRoute({
    getParentRoute: () => auth,
    path: '/usage-logs/$section',
    component: () => (
      <UsageLogsProvider>
        <CommonLogsStats />
      </UsageLogsProvider>
    ),
    validateSearch: (search: Record<string, unknown>) => search,
  })
  const router = createRouter({
    routeTree: root.addChildren([auth.addChildren([logs])]),
    history: createMemoryHistory({ initialEntries: ['/usage-logs/common'] }),
  })
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
  expect(await screen.findByText('Legacy usage')).toBeVisible()
  expect(screen.getByText('Legacy credits 1')).toBeVisible()
  expect(screen.getByText('点 2')).toBeVisible()
})
