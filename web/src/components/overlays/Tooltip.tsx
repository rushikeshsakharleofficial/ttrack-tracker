import React, { useState, useRef, cloneElement } from 'react'
import styles from './Tooltip.module.css'

type Placement = 'top' | 'bottom' | 'left' | 'right'
type TooltipVariant = 'dark' | 'light' | 'primary'

interface TooltipProps {
  content: React.ReactNode
  children: React.ReactElement
  placement?: Placement
  variant?: TooltipVariant
  delay?: number
  className?: string
}

export function Tooltip({
  content,
  children,
  placement = 'top',
  variant = 'dark',
  delay = 200,
  className,
}: TooltipProps) {
  const [visible, setVisible] = useState(false)
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null)

  const show = () => {
    timer.current = setTimeout(() => setVisible(true), delay)
  }
  const hide = () => {
    if (timer.current) clearTimeout(timer.current)
    setVisible(false)
  }

  const child = cloneElement(children, {
    onMouseEnter: (e: React.MouseEvent) => {
      show()
      children.props.onMouseEnter?.(e)
    },
    onMouseLeave: (e: React.MouseEvent) => {
      hide()
      children.props.onMouseLeave?.(e)
    },
    onFocus: (e: React.FocusEvent) => {
      show()
      children.props.onFocus?.(e)
    },
    onBlur: (e: React.FocusEvent) => {
      hide()
      children.props.onBlur?.(e)
    },
  })

  return (
    <span className={styles.wrapper}>
      {child}
      <span
        role="tooltip"
        className={[
          styles.tooltip,
          styles[`placement-${placement}`],
          styles[`variant-${variant}`],
          visible ? styles.visible : '',
          className,
        ]
          .filter(Boolean)
          .join(' ')}
      >
        {content}
        <span className={[styles.arrow, styles[`arrow-${placement}`]].join(' ')} />
      </span>
    </span>
  )
}
