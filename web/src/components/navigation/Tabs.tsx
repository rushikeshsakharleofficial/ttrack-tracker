import React, { useRef } from 'react'
import styles from './Tabs.module.css'

interface Tab {
  id: string
  label: string
  disabled?: boolean
  badge?: string | number
}

interface TabsProps {
  tabs: Tab[]
  activeTab: string
  onChange: (id: string) => void
  variant?: 'line' | 'pill'
  size?: 'sm' | 'md' | 'lg'
  className?: string
}

export function Tabs({
  tabs,
  activeTab,
  onChange,
  variant = 'line',
  size = 'md',
  className,
}: TabsProps) {
  const listRef = useRef<HTMLDivElement>(null)

  const handleKeyDown = (e: React.KeyboardEvent, index: number) => {
    const enabled = tabs.filter((t) => !t.disabled)
    const currentEnabled = enabled.findIndex((t) => t.id === tabs[index].id)
    let next = -1

    if (e.key === 'ArrowRight') next = (currentEnabled + 1) % enabled.length
    else if (e.key === 'ArrowLeft') next = (currentEnabled - 1 + enabled.length) % enabled.length
    else if (e.key === 'Home') next = 0
    else if (e.key === 'End') next = enabled.length - 1

    if (next >= 0) {
      e.preventDefault()
      const targetId = enabled[next].id
      onChange(targetId)
      const btn = listRef.current?.querySelector<HTMLButtonElement>(`[data-id="${targetId}"]`)
      btn?.focus()
    }
  }

  const cls = [
    styles.tabs,
    styles[`variant-${variant}`],
    styles[`size-${size}`],
    className,
  ]
    .filter(Boolean)
    .join(' ')

  return (
    <div className={cls}>
      <div role="tablist" ref={listRef} className={styles.list}>
        {tabs.map((tab, i) => {
          const active = tab.id === activeTab
          return (
            <button
              key={tab.id}
              role="tab"
              data-id={tab.id}
              aria-selected={active}
              aria-disabled={tab.disabled}
              disabled={tab.disabled}
              tabIndex={active ? 0 : -1}
              className={[styles.tab, active ? styles.active : ''].filter(Boolean).join(' ')}
              onClick={() => !tab.disabled && onChange(tab.id)}
              onKeyDown={(e) => handleKeyDown(e, i)}
            >
              {tab.label}
              {tab.badge != null && (
                <span className={styles.badge}>{tab.badge}</span>
              )}
            </button>
          )
        })}
      </div>
    </div>
  )
}
