import React, { useEffect } from 'react'
import styles from './Toast.module.css'

type ToastVariant = 'success' | 'warning' | 'danger' | 'info'

interface ToastProps {
  variant?: ToastVariant
  title?: string
  message: string
  onClose?: () => void
  duration?: number
  className?: string
}

const iconColors: Record<ToastVariant, string> = {
  success: '#10B981',
  warning: '#F59E0B',
  danger: '#EF4444',
  info: '#3B82F6',
}

function ToastIcon({ variant }: { variant: ToastVariant }) {
  const color = iconColors[variant]
  if (variant === 'success') {
    return (
      <svg width="16" height="16" viewBox="0 0 16 16" fill="none" aria-hidden="true">
        <path d="M3 8l4 4 6-6" stroke={color} strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" />
      </svg>
    )
  }
  if (variant === 'info') {
    return (
      <svg width="16" height="16" viewBox="0 0 16 16" fill="none" aria-hidden="true">
        <path d="M8 7v5M8 5h.01" stroke={color} strokeWidth="1.5" strokeLinecap="round" />
      </svg>
    )
  }
  return (
    <svg width="16" height="16" viewBox="0 0 16 16" fill="none" aria-hidden="true">
      <path d="M8 5v4M8 11h.01" stroke={color} strokeWidth="1.5" strokeLinecap="round" />
    </svg>
  )
}

export function Toast({
  variant = 'info',
  title,
  message,
  onClose,
  duration = 4000,
  className,
}: ToastProps) {
  useEffect(() => {
    if (!duration || !onClose) return
    const id = setTimeout(onClose, duration)
    return () => clearTimeout(id)
  }, [duration, onClose])

  const iconColor = iconColors[variant]

  return (
    <div
      className={[styles.toast, className ?? ''].filter(Boolean).join(' ')}
      role="alert"
      aria-live="assertive"
    >
      {/* ponytail: inline style only for dynamic icon circle color */}
      <span className={styles.iconCircle} style={{ background: `${iconColor}22`, color: iconColor }}>
        <ToastIcon variant={variant} />
      </span>
      <div className={styles.content}>
        {title && <p className={styles.title}>{title}</p>}
        <p className={styles.message}>{message}</p>
      </div>
      {onClose && (
        <button
          className={styles.close}
          onClick={onClose}
          aria-label="Dismiss"
          type="button"
        >
          <svg width="14" height="14" viewBox="0 0 14 14" fill="none" aria-hidden="true">
            <path d="M3 3l8 8M11 3l-8 8" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" />
          </svg>
        </button>
      )}
    </div>
  )
}

export default Toast
