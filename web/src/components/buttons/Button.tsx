import React from 'react'
import styles from './Button.module.css'

type Variant = 'primary' | 'secondary' | 'outline' | 'ghost' | 'danger'
type Size = 'sm' | 'md' | 'lg'

interface ButtonProps {
  variant?: Variant
  size?: Size
  disabled?: boolean
  loading?: boolean
  leftIcon?: React.ReactNode
  rightIcon?: React.ReactNode
  onClick?: () => void
  type?: 'button' | 'submit' | 'reset'
  children: React.ReactNode
  className?: string
  fullWidth?: boolean
}

export function Button({
  variant = 'primary',
  size = 'md',
  disabled = false,
  loading = false,
  leftIcon,
  rightIcon,
  onClick,
  type = 'button',
  children,
  className,
  fullWidth = false,
}: ButtonProps) {
  const classes = [
    styles.btn,
    styles[variant],
    styles[size],
    disabled ? styles.disabled : '',
    loading ? styles.loading : '',
    fullWidth ? styles.fullWidth : '',
    className ?? '',
  ]
    .filter(Boolean)
    .join(' ')

  return (
    <button
      type={type}
      className={classes}
      onClick={onClick}
      disabled={disabled || loading}
      aria-disabled={disabled || loading}
      aria-busy={loading}
    >
      {loading ? (
        <span className={styles.spinner} aria-hidden="true" />
      ) : (
        leftIcon
      )}
      {children}
      {!loading && rightIcon}
    </button>
  )
}

export default Button
