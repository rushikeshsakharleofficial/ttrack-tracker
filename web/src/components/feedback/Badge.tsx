import React from 'react'
import styles from './Badge.module.css'

type BadgeVariant = 'primary' | 'secondary' | 'success' | 'warning' | 'danger' | 'info' | 'neutral'
type BadgeSize = 'sm' | 'md'
type BadgeShape = 'pill' | 'square'

interface BadgeProps {
  variant?: BadgeVariant
  size?: BadgeSize
  shape?: BadgeShape
  dot?: boolean
  children: React.ReactNode
  className?: string
}

export function Badge({
  variant = 'primary',
  size = 'md',
  shape = 'pill',
  dot = false,
  children,
  className,
}: BadgeProps) {
  const classes = [
    styles.badge,
    styles[variant],
    styles[size],
    styles[shape],
    className ?? '',
  ]
    .filter(Boolean)
    .join(' ')

  return (
    <span className={classes}>
      {dot && <span className={styles.dot} aria-hidden="true" />}
      {children}
    </span>
  )
}

export default Badge
