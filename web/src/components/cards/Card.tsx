import React from 'react'
import styles from './Card.module.css'

type Variant = 'default' | 'bordered' | 'elevated'
type Padding = 'none' | 'sm' | 'md' | 'lg'
type Radius = 'sm' | 'md' | 'lg'

interface CardProps {
  variant?: Variant
  padding?: Padding
  radius?: Radius
  children: React.ReactNode
  className?: string
  onClick?: () => void
  header?: React.ReactNode
  footer?: React.ReactNode
}

export function Card({
  variant = 'default',
  padding = 'md',
  radius = 'md',
  children,
  className,
  onClick,
  header,
  footer,
}: CardProps) {
  const classes = [
    styles.card,
    styles[variant],
    styles[`radius-${radius}`],
    onClick ? styles.clickable : '',
    className ?? '',
  ]
    .filter(Boolean)
    .join(' ')

  return (
    <div
      className={classes}
      onClick={onClick}
      role={onClick ? 'button' : undefined}
      tabIndex={onClick ? 0 : undefined}
      onKeyDown={onClick ? (e) => e.key === 'Enter' && onClick() : undefined}
    >
      {header && <div className={styles.header}>{header}</div>}
      <div className={`${styles.body} ${styles[`padding-${padding}`]}`}>
        {children}
      </div>
      {footer && <div className={styles.footer}>{footer}</div>}
    </div>
  )
}

export default Card
