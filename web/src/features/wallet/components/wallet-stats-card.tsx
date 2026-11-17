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
import { Activity, BarChart3, WalletCards } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { IconBadge, type IconBadgeTone } from '@/components/ui/icon-badge'
import { Skeleton } from '@/components/ui/skeleton'
import {
  formatCumulativeQuota,
  formatQuotaWithCurrency,
  getCurrentUsedQuota,
} from '@/lib/currency'
import { cn } from '@/lib/utils'
import { useSystemConfigStore } from '@/stores/system-config-store'

import type { UserWalletData } from '../types'

interface WalletStatsCardProps {
  user: UserWalletData | null
  loading?: boolean
}

export function WalletStatsCard(props: WalletStatsCardProps) {
  const { t } = useTranslation()
  const config = useSystemConfigStore((state) => state.config)
  if (props.loading) {
    return (
      <div className='grid grid-cols-3 divide-x rounded-lg border'>
        {['balance', 'usage', 'requests'].map((key) => (
          <div key={key} className='min-w-0 px-2.5 py-2.5 sm:px-5 sm:py-4'>
            <Skeleton className='h-3.5 w-full' />
            <Skeleton className='mt-2 h-6 w-full sm:h-7' />
            <Skeleton className='mt-1.5 hidden h-3.5 w-24 md:block' />
          </div>
        ))}
      </div>
    )
  }

  const currency = config.currency
  const unit =
    currency?.quotaDisplayType === 'CUSTOM' &&
    ['点', '點'].includes(currency.customCurrencySymbol)
      ? currency.customCurrencySymbol
      : undefined
  const quotaOptions = { showSymbol: !unit }
  const total = props.user?.used_quota ?? 0
  const legacy = props.user?.legacy_used_quota
  const current = getCurrentUsedQuota(total, legacy)
  const hasHistory = current != null && current !== total && legacy != null

  const stats: {
    label: string
    value: string
    unit?: string
    showDescriptionOnMobile?: boolean
    description: string
    icon: typeof WalletCards
    tone: IconBadgeTone
  }[] = [
    {
      label: t('Current Balance'),
      value: formatQuotaWithCurrency(props.user?.quota ?? 0, quotaOptions),
      unit,
      description: t('Remaining quota'),
      icon: WalletCards,
      tone: 'success',
    },
    {
      label: hasHistory ? t('Usage since migration') : t('Total Usage'),
      value:
        current == null
          ? formatCumulativeQuota(total, legacy)
          : formatQuotaWithCurrency(current, quotaOptions),
      unit: current == null ? undefined : unit,
      description: hasHistory
        ? t('Previously used {{amount}} legacy credits', {
            amount: formatQuotaWithCurrency(legacy, {
              recordedAt: 0,
              showSymbol: false,
            }),
          })
        : t('Total consumed quota'),
      showDescriptionOnMobile: hasHistory,
      icon: BarChart3,
      tone: 'info',
    },
    {
      label: t('API Requests'),
      value: (props.user?.request_count ?? 0).toLocaleString(),
      description: t('Total requests made'),
      icon: Activity,
      tone: 'chart-4',
    },
  ]

  return (
    <dl className='grid grid-cols-3 divide-x rounded-lg border'>
      {stats.map((item) => (
        <div key={item.label} className='min-w-0 px-2.5 py-2.5 sm:px-5 sm:py-4'>
          <dt className='flex items-center gap-1.5 sm:gap-2.5'>
            <IconBadge tone={item.tone} size='stat'>
              <item.icon />
            </IconBadge>
            <div className='text-muted-foreground truncate text-[11px] font-medium tracking-wider uppercase sm:text-xs'>
              {item.label}
            </div>
          </dt>

          <dd className='text-foreground mt-1.5 text-sm font-semibold tracking-tight break-all tabular-nums sm:mt-2.5 sm:text-2xl'>
            {item.value}
            {item.unit && (
              <span className='text-muted-foreground whitespace-nowrap font-sans text-xs font-medium tracking-normal sm:text-sm'>
                {' '}
                {item.unit}
              </span>
            )}
          </dd>
          <dd
            className={cn(
              'text-muted-foreground mt-1 text-xs leading-snug',
              !item.showDescriptionOnMobile && 'hidden md:block'
            )}
          >
            {item.description}
          </dd>
        </div>
      ))}
    </dl>
  )
}
