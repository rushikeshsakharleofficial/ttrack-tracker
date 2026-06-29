import React from 'react'
import styles from './Breadcrumb.module.css'

interface BreadcrumbItem {
  label: string
  href?: string
  onClick?: () => void
}

interface BreadcrumbProps {
  items: BreadcrumbItem[]
  separator?: React.ReactNode
  className?: string
}

export function Breadcrumb({
  items,
  separator = '/',
  className,
}: BreadcrumbProps) {
  return (
    <nav aria-label="breadcrumb" className={[styles.breadcrumb, className].filter(Boolean).join(' ')}>
      <ol className={styles.list}>
        {items.map((item, i) => {
          const isLast = i === items.length - 1
          return (
            <li key={i} className={styles.item}>
              {i > 0 && (
                <span className={styles.separator} aria-hidden="true">
                  {separator}
                </span>
              )}
              {isLast ? (
                <span className={styles.current} aria-current="page">
                  {item.label}
                </span>
              ) : item.href ? (
                <a href={item.href} className={styles.link}>
                  {item.label}
                </a>
              ) : (
                <button
                  type="button"
                  className={styles.link}
                  onClick={item.onClick}
                >
                  {item.label}
                </button>
              )}
            </li>
          )
        })}
      </ol>
    </nav>
  )
}
