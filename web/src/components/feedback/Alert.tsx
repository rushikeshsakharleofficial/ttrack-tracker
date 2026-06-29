import React from 'react'
import styles from './Alert.module.css'

type AlertVariant = 'success' | 'warning' | 'danger' | 'info'

interface AlertProps {
  variant: AlertVariant
  title?: string
  children: React.ReactNode
  onClose?: () => void
  className?: string
}

function Icon({ variant }: { variant: AlertVariant }) {
  if (variant === 'success') {
    return (
      <svg width="20" height="20" viewBox="0 0 20 20" fill="none" aria-hidden="true">
        <circle cx="10" cy="10" r="10" fill="#10B981" />
        <path d="M6 10l3 3 5-5" stroke="#fff" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" />
      </svg>
    )
  }
  if (variant === 'info') {
    return (
      <svg width="20" height="20" viewBox="0 0 20 20" fill="none" aria-hidden="true">
        <circle cx="10" cy="10" r="10" fill="#3B82F6" />
        <path d="M10 9v5M10 7h.01" stroke="#fff" strokeWidth="1.5" strokeLinecap="round" />
      </svg>
    )
  }
  const fill = variant === 'warning' ? '#F59E0B' : '#EF4444'
  return (
    <svg width="20" height="20" viewBox="0 0 20 20" fill="none" aria-hidden="true">
      <circle cx="10" cy="10" r="10" fill={fill} />
      <path d="M10 6v5M10 13h.01" stroke="#fff" strokeWidth="1.5" strokeLinecap="round" />
    </svg>
  )
}

export function Alert({ variant, title, children, onClose, className }: AlertProps) {
  return (
    <div
      className={[styles.alert, styles[variant], className ?? ''].filter(Boolean).join(' ')}
      role="alert"
    >
      <span className={styles.icon}>
        <Icon variant={variant} />
      </span>
      <div className={styles.content}>
        {title && <p className={styles.title}>{title}</p>}
        <div className={styles.body}>{children}</div>
      </div>
      {onClose && (
        <button
          className={styles.close}
          onClick={onClose}
          aria-label="Dismiss alert"
          type="button"
        >
          <svg width="16" height="16" viewBox="0 0 16 16" fill="none" aria-hidden="true">
            <path d="M4 4l8 8M12 4l-8 8" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" />
          </svg>
        </button>
      )}
    </div>
  )
}

export default Alert
