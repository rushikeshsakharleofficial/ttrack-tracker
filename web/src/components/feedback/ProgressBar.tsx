import React from 'react'
import styles from './ProgressBar.module.css'

type ProgressVariant = 'primary' | 'success' | 'warning' | 'danger'
type ProgressSize = 'sm' | 'md' | 'lg'

interface ProgressBarProps {
  value: number
  max?: number
  variant?: ProgressVariant
  size?: ProgressSize
  label?: string
  showValue?: boolean
  animated?: boolean
  striped?: boolean
  className?: string
}

export function ProgressBar({
  value,
  max = 100,
  variant = 'primary',
  size = 'md',
  label,
  showValue = false,
  animated = false,
  striped = false,
  className,
}: ProgressBarProps) {
  const pct = Math.min(100, Math.max(0, (value / max) * 100))

  const fillClasses = [
    styles.fill,
    styles[variant],
    animated ? styles.animated : '',
    striped ? styles.striped : '',
  ]
    .filter(Boolean)
    .join(' ')

  return (
    <div className={[styles.wrapper, className ?? ''].filter(Boolean).join(' ')}>
      {(label || showValue) && (
        <div className={styles.labelRow}>
          {label && <span className={styles.label}>{label}</span>}
          {showValue && <span className={styles.value}>{Math.round(pct)}%</span>}
        </div>
      )}
      <div
        className={[styles.track, styles[size]].join(' ')}
        role="progressbar"
        aria-valuenow={value}
        aria-valuemin={0}
        aria-valuemax={max}
        aria-label={label}
      >
        {/* ponytail: inline style only for dynamic width — CSS variable alternative would need JS anyway */}
        <div className={fillClasses} style={{ width: `${pct}%` }} />
      </div>
    </div>
  )
}

export default ProgressBar
